// Package agent provides the unified Agent type.
//
// Every agent — Mind, CTO, Guardian, Builder, etc. — is an Agent.
// The Agent wraps EventGraph's AgentRuntime and adds:
//   - Operational state machine (driven on every operation)
//   - True causality tracking (each event caused by the previous, not store.Head())
//   - Trust accumulation hooks
//   - Budget event emission
//   - Proper retirement lifecycle
//   - Communication through the shared graph
//
// Construction:
//
//	a, err := agent.New(ctx, agent.Config{
//	    Role:       "cto",
//	    Name:       "CTO",
//	    Graph:      g,
//	    Provider:   provider,
//	    Model:      "claude-opus-4-6",
//	    CostTier:   "opus",
//	    SoulValues: []string{"Take care of your human..."},
//	})
package agent

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"sync"

	egagent "github.com/lovyou-ai/eventgraph/go/pkg/agent"
	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/graph"
	"github.com/lovyou-ai/eventgraph/go/pkg/intelligence"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"
)

// Role identifies an agent's function. Constants are defined by the
// application layer (e.g. hive/pkg/roles), not here — this package
// is role-agnostic.
type Role string

// Agent is the unified agent type.
// All agent operations go through this type, which ensures proper state
// transitions, event emission, causality tracking, and trust accumulation.
type Agent struct {
	runtime *intelligence.AgentRuntime
	role    Role
	name    string
	graph   *graph.Graph
	convID  types.ConversationID
	state   egagent.OperationalState
	signer  event.Signer

	// lastEvent tracks the most recent event this agent emitted.
	// Used for true causality — each event is caused by the previous,
	// not by whatever happens to be at store.Head().
	lastEvent types.EventID

	mu sync.Mutex
}

// Config holds the configuration needed to create an Agent.
// Role-specific data (model, soul values) is passed explicitly
// rather than derived — the agent package is role-agnostic.
type Config struct {
	Role     Role
	Name     string
	Graph    *graph.Graph
	Provider intelligence.Provider

	// Model is the LLM model identifier for the agent boot sequence.
	Model string

	// CostTier is the cost classification (e.g. "opus", "standard").
	CostTier string

	// SoulValues are the agent's core values, emitted during boot.
	SoulValues []string

	// ConversationID overrides the default agent-scoped conversation ID.
	// When set, the agent's events join this conversation thread.
	// Used by the pipeline to unify all agents under one conversation.
	ConversationID types.ConversationID
}

// New creates and boots a new Agent.
//
// The agent is registered in the actor store, its signing key is derived
// deterministically from its name, and the full boot sequence is emitted
// (identity, soul, model, authority, state → Idle).
//
// Requires: cfg.Graph.Start() must be called before New(). The boot
// sequence emits events via graph.Record(), which requires a started graph.
func New(ctx context.Context, cfg Config) (*Agent, error) {
	if cfg.Graph == nil {
		return nil, fmt.Errorf("agent: Graph is required")
	}
	if cfg.Provider == nil {
		return nil, fmt.Errorf("agent: Provider is required")
	}

	// Derive deterministic signing key from agent name.
	// NOTE: Names must be unique within a deployment. Two agents with the
	// same name will produce identical keys — their signatures become
	// indistinguishable on the graph. For cross-deployment uniqueness,
	// include a deployment-specific salt in the name.
	seed := sha256.Sum256([]byte("agent:" + cfg.Name))
	priv := ed25519.NewKeyFromSeed(seed[:])
	pub := priv.Public().(ed25519.PublicKey)
	signer := &deterministicSigner{key: priv}

	// Register in actor store.
	pk, err := types.NewPublicKey([]byte(pub))
	if err != nil {
		return nil, fmt.Errorf("agent: public key: %w", err)
	}

	registered, err := cfg.Graph.ActorStore().Register(pk, cfg.Name, event.ActorTypeAI)
	if err != nil {
		// Actor may already exist from a previous run — look it up.
		existing, getErr := cfg.Graph.ActorStore().GetByPublicKey(pk)
		if getErr != nil {
			return nil, fmt.Errorf("agent: register %s: %w", cfg.Name, err)
		}
		registered = existing
	}

	// Create the AgentRuntime.
	// Use configured conversation ID if provided; otherwise derive from agent ID.
	convID := cfg.ConversationID
	if convID.Value() == "" {
		convID = types.MustConversationID(fmt.Sprintf("agent_%s", registered.ID().Value()))
	}
	rt, err := intelligence.NewRuntime(ctx, intelligence.RuntimeConfig{
		AgentID:        registered.ID(),
		Provider:       cfg.Provider,
		Store:          cfg.Graph.Store(),
		ConversationID: convID,
	})
	if err != nil {
		return nil, fmt.Errorf("agent: runtime: %w", err)
	}

	a := &Agent{
		runtime: rt,
		role:    cfg.Role,
		name:    cfg.Name,
		graph:   cfg.Graph,
		convID:  convID,
		state:   egagent.StateIdle,
		signer:  signer,
	}

	// Boot: emit lifecycle events on the graph.
	if err := a.boot(pk, cfg); err != nil {
		return nil, fmt.Errorf("agent: boot %s: %w", cfg.Name, err)
	}

	return a, nil
}

// boot emits the full agent lifecycle boot sequence.
func (a *Agent) boot(pk types.PublicKey, cfg Config) error {
	model := cfg.Model
	if model == "" {
		model = cfg.Provider.Model()
	}
	costTier := cfg.CostTier
	if costTier == "" {
		costTier = "standard"
	}

	bootEvents := egagent.BootEvents(
		a.runtime.ID(),
		pk,
		string(cfg.Role),
		model,
		costTier,
		cfg.SoulValues,
		types.MustDomainScope("hive"),
		a.runtime.ID(), // self-grant at boot; human approves via Spawner
		false,          // withIdentity=false, Spawner handles identity
	)

	for _, content := range bootEvents {
		_, err := a.recordAndTrack(content.EventTypeName(), content)
		if err != nil {
			return fmt.Errorf("boot event %s: %w", content.EventTypeName(), err)
		}
	}

	return nil
}

// record emits an event through the Graph facade (mutex-safe, bus-integrated).
// Caller must hold a.mu. Called by recordAndTrack() and transitionLocked().
func (a *Agent) record(eventTypeName string, content event.EventContent) (event.Event, error) {
	eventType := types.MustEventType(eventTypeName)

	var causes []types.EventID
	if !a.lastEvent.IsZero() {
		causes = []types.EventID{a.lastEvent}
	} else {
		// First event — use graph head as cause.
		head, err := a.graph.Store().Head()
		if err != nil {
			return event.Event{}, fmt.Errorf("get head: %w", err)
		}
		if head.IsSome() {
			causes = []types.EventID{head.Unwrap().ID()}
		}
	}

	return a.graph.Record(eventType, a.runtime.ID(), content, causes, a.convID, a.signer)
}

// recordAndTrack atomically records an event and updates lastEvent.
// Holds a.mu for the entire operation to prevent causality races.
func (a *Agent) recordAndTrack(eventTypeName string, content event.EventContent) (event.Event, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	ev, err := a.record(eventTypeName, content)
	if err != nil {
		return ev, err
	}
	a.lastEvent = ev.ID()
	return ev, nil
}

// checkCanEmit returns an error if the agent is in a terminal or suspended
// state. Methods that emit events without driving the FSM (Learn, Act,
// Communicate, Introspect, etc.) must call this first — otherwise a retired
// or suspended agent could emit events after its memorial/halt.
func (a *Agent) checkCanEmit() error {
	a.mu.Lock()
	s := a.state
	a.mu.Unlock()
	switch s {
	case egagent.StateRetired:
		return fmt.Errorf("agent is retired")
	case egagent.StateSuspended:
		return fmt.Errorf("agent is suspended")
	default:
		return nil
	}
}

// --- Accessors ---

// ID returns the agent's actor ID.
func (a *Agent) ID() types.ActorID { return a.runtime.ID() }

// Role returns the agent's role.
func (a *Agent) Role() Role { return a.role }

// Name returns the agent's display name.
func (a *Agent) Name() string { return a.name }

// State returns the agent's current operational state.
func (a *Agent) State() egagent.OperationalState {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.state
}

// Runtime returns the underlying AgentRuntime for advanced use.
// Prefer the Agent's methods over direct Runtime access.
func (a *Agent) Runtime() *intelligence.AgentRuntime { return a.runtime }

// Provider returns the agent's intelligence provider.
func (a *Agent) Provider() intelligence.Provider { return a.runtime.Provider() }

// Graph returns the shared graph.
func (a *Agent) Graph() *graph.Graph { return a.graph }

// ConversationID returns the agent's conversation thread ID.
func (a *Agent) ConversationID() types.ConversationID { return a.convID }

// LastEvent returns the ID of the most recent event this agent emitted.
func (a *Agent) LastEvent() types.EventID {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lastEvent
}

// deterministicSigner signs with a key derived from the agent name.
type deterministicSigner struct {
	key ed25519.PrivateKey
}

func (s *deterministicSigner) Sign(data []byte) (types.Signature, error) {
	sig := ed25519.Sign(s.key, data)
	return types.NewSignature(sig)
}
