package agent

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/actor"
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
