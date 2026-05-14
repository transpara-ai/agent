package agent

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/actor"
	v39 "github.com/transpara-ai/eventgraph/go/pkg/darkfactory/v39"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/graph"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

func newStartedGraph(t *testing.T) *graph.Graph {
	t.Helper()
	g := graph.New(store.NewInMemoryStore(), actor.NewInMemoryActorStore())
	if err := g.Start(); err != nil {
		t.Fatalf("graph.Start: %v", err)
	}
	t.Cleanup(func() { _ = g.Close() })
	return g
}

func testConfig(g *graph.Graph, name string) Config {
	return Config{
		Role:     "test",
		Name:     name,
		Graph:    g,
		Provider: &mockProvider{},
		Model:    "mock-model",
		CostTier: "standard",
	}
}

func deterministicPublicKey(name string) []byte {
	return []byte(deterministicPrivateKey(name).Public().(ed25519.PublicKey))
}

func deterministicPrivateKey(name string) ed25519.PrivateKey {
	seed := sha256.Sum256([]byte("agent:" + name))
	return ed25519.NewKeyFromSeed(seed[:])
}

func deterministicActorID(name string) string {
	h := sha256.Sum256(deterministicPublicKey(name))
	return fmt.Sprintf("actor_%s", hex.EncodeToString(h[:16]))
}

func testAuthority(t *testing.T, action, target string) *IdentityAuthority {
	t.Helper()
	now := time.Now().UTC()
	status := "active"
	reqID := fmt.Sprintf("req_%s_%s", strings.ReplaceAll(action, ".", "_"), target)
	return &IdentityAuthority{
		Request: v39.AuthorityRequest{
			CommonNode: v39.CommonNode{
				ID:             reqID,
				Type:           v39.TypeAuthorityRequest,
				CreatedAt:      now,
				CreatedBy:      "actor_operator",
				Status:         &status,
				IdempotencyKey: "idem_" + reqID,
				CorrelationID:  "corr_" + target,
			},
			ActorID:      "actor_operator",
			ActorRole:    "Operator",
			Action:       action,
			TargetType:   "agent_identity",
			TargetID:     target,
			RiskClass:    "high",
			Reason:       "test approval",
			EvidenceRefs: []string{"test"},
		},
		Decision: v39.AuthorityDecision{
			CommonNode: v39.CommonNode{
				ID:             "dec_" + reqID,
				Type:           v39.TypeAuthorityDecision,
				CreatedAt:      now,
				CreatedBy:      "actor_operator",
				Status:         &status,
				IdempotencyKey: "idem_dec_" + reqID,
				CorrelationID:  "corr_" + target,
			},
			AuthorityRequestID: reqID,
			DeciderActorID:     "actor_operator",
			DeciderRole:        "Operator",
			Decision:           "Autonomous",
			Reason:             "approved for test",
			Scope:              []string{target},
			Conditions:         []string{"local test only"},
		},
	}
}

func persistentConfig(g *graph.Graph, name, ref string, store IdentityStore, recordStore IdentityRecordStore) Config {
	cfg := testConfig(g, name)
	cfg.IdentityStore = store
	cfg.PersistentIdentityRef = ref
	cfg.IdentityRecordStore = recordStore
	return cfg
}

func TestProductionRejectsDeterministicIdentity(t *testing.T) {
	cfg := testConfig(newStartedGraph(t), "PublicName")
	cfg.IdentityMode = IdentityModeDeterministic

	_, err := New(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected deterministic production identity to be rejected")
	}
	if !strings.Contains(err.Error(), "deterministic identity is blocked in production") {
		t.Fatalf("error = %q, want production deterministic block", err.Error())
	}
}

func TestProductionRejectsSuppliedPublicNameDerivedSigningKey(t *testing.T) {
	name := "PublicName"
	cfg := testConfig(newStartedGraph(t), name)
	cfg.SigningKey = deterministicPrivateKey(name)

	_, err := New(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected supplied public-name-derived production identity to be rejected")
	}
	if !strings.Contains(err.Error(), "public-name-derived identity is blocked in production") {
		t.Fatalf("error = %q, want supplied deterministic key block", err.Error())
	}
}

func TestProductionGeneratedIdentityDoesNotUsePublicNameSeed(t *testing.T) {
	name := "PublicName"
	a, err := New(context.Background(), testConfig(newStartedGraph(t), name))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	identity := identityCreatedContent(t, a)
	if bytes.Equal(identity.PublicKey.Bytes(), deterministicPublicKey(name)) {
		t.Fatal("production public key matches sha256(\"agent:\"+name) derived identity")
	}
}

func TestProductionRestartReusesPersistentIdentity(t *testing.T) {
	ctx := context.Background()
	ref := "prod-agent"
	store := NewFileIdentityStore(t.TempDir())
	records := v39.NewInMemoryStore()

	firstGraph := newStartedGraph(t)
	firstCfg := persistentConfig(firstGraph, "PersistentAgent", ref, store, records)
	firstCfg.CreatePersistentIdentity = true
	firstCfg.IdentityAuthority = testAuthority(t, identityActionSpawnPersistent, ref)
	first, err := New(ctx, firstCfg)
	if err != nil {
		t.Fatalf("first New: %v", err)
	}

	secondGraph := newStartedGraph(t)
	secondCfg := persistentConfig(secondGraph, "PersistentAgent", ref, store, records)
	second, err := New(ctx, secondCfg)
	if err != nil {
		t.Fatalf("second New: %v", err)
	}

	if first.ID() != second.ID() {
		t.Fatalf("restart actor ID = %s, want %s", second.ID().Value(), first.ID().Value())
	}
	firstIdentity := identityCreatedContent(t, first)
	secondIdentity := identityCreatedContent(t, second)
	if !bytes.Equal(firstIdentity.PublicKey.Bytes(), secondIdentity.PublicKey.Bytes()) {
		t.Fatal("restart did not reuse persisted public key")
	}
}

func TestMissingPersistentProductionIdentityFailsClosed(t *testing.T) {
	cfg := persistentConfig(newStartedGraph(t), "MissingPersistent", "missing-agent", NewInMemoryIdentityStore(), v39.NewInMemoryStore())

	_, err := New(context.Background(), cfg)
	if !errors.Is(err, ErrIdentityNotFound) {
		t.Fatalf("New error = %v, want ErrIdentityNotFound", err)
	}
}

func TestExplicitPersistentProductionIdentityCreationRequiresAuthority(t *testing.T) {
	ctx := context.Background()
	ref := "created-agent"
	store := NewInMemoryIdentityStore()
	records := v39.NewInMemoryStore()

	cfg := persistentConfig(newStartedGraph(t), "CreatedAgent", ref, store, records)
	cfg.CreatePersistentIdentity = true
	_, err := New(ctx, cfg)
	if !errors.Is(err, ErrAuthorityRequired) {
		t.Fatalf("New without authority error = %v, want ErrAuthorityRequired", err)
	}

	cfg = persistentConfig(newStartedGraph(t), "CreatedAgent", ref, store, records)
	cfg.CreatePersistentIdentity = true
	cfg.IdentityAuthority = testAuthority(t, identityActionSpawnPersistent, ref)
	a, err := New(ctx, cfg)
	if err != nil {
		t.Fatalf("New with authority: %v", err)
	}
	if a.ID().Value() == deterministicActorID("CreatedAgent") {
		t.Fatal("persistent production identity used deterministic fixture actor ID")
	}
	if len(records.ByType(v39.TypeActorIdentity)) != 1 {
		t.Fatalf("ActorIdentity records = %d, want 1", len(records.ByType(v39.TypeActorIdentity)))
	}
	if len(records.ByType(v39.TypeExecutionReceipt)) != 1 {
		t.Fatalf("ExecutionReceipt records = %d, want 1", len(records.ByType(v39.TypeExecutionReceipt)))
	}
}

func TestProductionPersistentCreationRejectsDeterministicSigningKey(t *testing.T) {
	cfg := persistentConfig(newStartedGraph(t), "PublicName", "deterministic-agent", NewInMemoryIdentityStore(), v39.NewInMemoryStore())
	cfg.CreatePersistentIdentity = true
	cfg.SigningKey = deterministicPrivateKey("PublicName")
	cfg.IdentityAuthority = testAuthority(t, identityActionSpawnPersistent, "deterministic-agent")

	_, err := New(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "public-name-derived identity is blocked in production") {
		t.Fatalf("New error = %v, want deterministic key block", err)
	}
}

func TestRotatePersistentIdentityChangesKeyMaterialAndRecordsContinuity(t *testing.T) {
	ctx := context.Background()
	ref := "rotate-agent"
	store := NewInMemoryIdentityStore()
	records := v39.NewInMemoryStore()
	created, err := RegisterPersistentIdentity(ctx, IdentityOperation{
		Store:       store,
		RecordStore: records,
		Ref:         ref,
		Name:        "RotateAgent",
		Authority:   testAuthority(t, identityActionSpawnPersistent, ref),
		Reason:      "create",
	})
	if err != nil {
		t.Fatalf("RegisterPersistentIdentity: %v", err)
	}

	rotated, err := RotatePersistentIdentity(ctx, IdentityOperation{
		Store:       store,
		RecordStore: records,
		Ref:         ref,
		Name:        "RotateAgent",
		Authority:   testAuthority(t, identityActionRotateKey, ref),
		Reason:      "rotate",
	})
	if err != nil {
		t.Fatalf("RotatePersistentIdentity: %v", err)
	}
	if rotated.Version != created.Version+1 {
		t.Fatalf("rotated version = %d, want %d", rotated.Version, created.Version+1)
	}
	if bytes.Equal(rotated.PublicKey, created.PublicKey) {
		t.Fatal("rotation reused previous public key")
	}
	if rotated.ActorID != created.ActorID {
		t.Fatalf("rotated actor ID = %s, want immutable identity %s", rotated.ActorID, created.ActorID)
	}
	if len(rotated.PreviousPublicKeys) != 1 || rotated.PreviousPublicKeys[0] != publicKeyRef(created.PublicKey) {
		t.Fatalf("previous key refs = %#v, want previous public key ref", rotated.PreviousPublicKeys)
	}
	if len(records.ByType(v39.TypeLifecycleTransition)) != 2 {
		t.Fatalf("LifecycleTransition records = %d, want 2", len(records.ByType(v39.TypeLifecycleTransition)))
	}
	if len(records.ByType(v39.TypeExecutionReceipt)) != 2 {
		t.Fatalf("ExecutionReceipt records = %d, want 2", len(records.ByType(v39.TypeExecutionReceipt)))
	}
}

func TestRotationAndRevocationRequireAuthority(t *testing.T) {
	ctx := context.Background()
	ref := "gated-agent"
	store := NewInMemoryIdentityStore()
	records := v39.NewInMemoryStore()
	_, err := RegisterPersistentIdentity(ctx, IdentityOperation{
		Store:       store,
		RecordStore: records,
		Ref:         ref,
		Name:        "GatedAgent",
		Authority:   testAuthority(t, identityActionSpawnPersistent, ref),
	})
	if err != nil {
		t.Fatalf("RegisterPersistentIdentity: %v", err)
	}

	_, err = RotatePersistentIdentity(ctx, IdentityOperation{
		Store:       store,
		RecordStore: records,
		Ref:         ref,
		Name:        "GatedAgent",
	})
	if !errors.Is(err, ErrAuthorityRequired) {
		t.Fatalf("RotatePersistentIdentity error = %v, want ErrAuthorityRequired", err)
	}
	err = RevokePersistentIdentity(ctx, IdentityOperation{
		Store:       store,
		RecordStore: records,
		Ref:         ref,
		Name:        "GatedAgent",
	})
	if !errors.Is(err, ErrAuthorityRequired) {
		t.Fatalf("RevokePersistentIdentity error = %v, want ErrAuthorityRequired", err)
	}
}

func TestRevokePersistentIdentityPreventsProductionUse(t *testing.T) {
	ctx := context.Background()
	ref := "revoke-agent"
	store := NewInMemoryIdentityStore()
	records := v39.NewInMemoryStore()
	_, err := RegisterPersistentIdentity(ctx, IdentityOperation{
		Store:       store,
		RecordStore: records,
		Ref:         ref,
		Name:        "RevokeAgent",
		Authority:   testAuthority(t, identityActionSpawnPersistent, ref),
	})
	if err != nil {
		t.Fatalf("RegisterPersistentIdentity: %v", err)
	}
	if err := RevokePersistentIdentity(ctx, IdentityOperation{
		Store:       store,
		RecordStore: records,
		Ref:         ref,
		Name:        "RevokeAgent",
		Authority:   testAuthority(t, identityActionRevoke, ref),
		Reason:      "revoke",
	}); err != nil {
		t.Fatalf("RevokePersistentIdentity: %v", err)
	}

	cfg := persistentConfig(newStartedGraph(t), "RevokeAgent", ref, store, records)
	_, err = New(ctx, cfg)
	if !errors.Is(err, ErrIdentityRevoked) {
		t.Fatalf("New revoked error = %v, want ErrIdentityRevoked", err)
	}
}

func TestExternallyManagedPersistentIdentityIsLoadedAfterRestart(t *testing.T) {
	_, supplied, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	ctx := context.Background()
	ref := "external-agent"
	store := NewInMemoryIdentityStore()
	records := v39.NewInMemoryStore()

	created, err := RegisterPersistentIdentity(ctx, IdentityOperation{
		Store:       store,
		RecordStore: records,
		Ref:         ref,
		Name:        "ExternalAgent",
		SigningKey:  supplied,
		Authority:   testAuthority(t, identityActionSpawnPersistent, ref),
	})
	if err != nil {
		t.Fatalf("RegisterPersistentIdentity: %v", err)
	}
	if created.Mode != IdentityModeExternallyManaged {
		t.Fatalf("created mode = %q, want externally managed", created.Mode)
	}

	cfg := persistentConfig(newStartedGraph(t), "ExternalAgent", ref, store, records)
	a, err := New(ctx, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if a.ID().Value() != created.ActorID {
		t.Fatalf("loaded actor = %s, want %s", a.ID().Value(), created.ActorID)
	}
}

func TestDeterministicIdentityAllowedOnlyWhenExplicitlyMarkedTest(t *testing.T) {
	name := "FixtureName"
	cfg := testConfig(newStartedGraph(t), name)
	cfg.Environment = IdentityEnvironmentTest
	cfg.IdentityMode = IdentityModeDeterministic

	a, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if a.ID().Value() != deterministicActorID(name) {
		t.Fatal("test deterministic identity did not use the explicit fixture key")
	}
}

func TestDeterministicIdentityAllowedOnlyWhenExplicitlyMarkedDevelopment(t *testing.T) {
	name := "DevFixtureName"
	cfg := testConfig(newStartedGraph(t), name)
	cfg.Environment = IdentityEnvironmentDevelopment
	cfg.IdentityMode = IdentityModeDeterministic

	a, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if a.ID().Value() != deterministicActorID(name) {
		t.Fatal("development deterministic identity did not use the explicit fixture key")
	}
}

func TestNewEmitsIdentityCreatedLifecycleEvent(t *testing.T) {
	g := newStartedGraph(t)
	a, err := New(context.Background(), testConfig(g, "LifecycleAgent"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	content := identityCreatedContent(t, a)
	if content.AgentID != a.ID() {
		t.Fatalf("identity AgentID = %s, want %s", content.AgentID.Value(), a.ID().Value())
	}
	registered, err := g.ActorStore().GetByPublicKey(content.PublicKey)
	if err != nil {
		t.Fatalf("GetByPublicKey(identity.PublicKey): %v", err)
	}
	if registered.ID() != a.ID() {
		t.Fatalf("registered actor = %s, want %s", registered.ID().Value(), a.ID().Value())
	}
}

func identityCreatedContent(t *testing.T, a *Agent) event.AgentIdentityCreatedContent {
	t.Helper()
	page, err := a.Graph().Store().ByType(event.EventTypeAgentIdentityCreated, 10, types.None[types.Cursor]())
	if err != nil {
		t.Fatalf("ByType(agent.identity.created): %v", err)
	}
	items := page.Items()
	if len(items) != 1 {
		t.Fatalf("identity event count = %d, want 1", len(items))
	}
	content, ok := items[0].Content().(event.AgentIdentityCreatedContent)
	if !ok {
		t.Fatalf("identity content type = %T", items[0].Content())
	}
	return content
}
