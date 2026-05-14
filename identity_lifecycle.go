package agent

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	v39 "github.com/transpara-ai/eventgraph/go/pkg/darkfactory/v39"
)

const (
	identityActionSpawnPersistent = "agent.spawn.persistent"
	identityActionRotateKey       = "agent.key.rotate"
	identityActionRevoke          = "agent.revoke"

	identityStatusActive  = "active"
	identityStatusRevoked = "revoked"
)

var (
	ErrIdentityNotFound      = errors.New("agent: persistent identity not found")
	ErrIdentityRevoked       = errors.New("agent: persistent identity is revoked")
	ErrIdentityRefRequired   = errors.New("agent: PersistentIdentityRef is required when IdentityStore is configured")
	ErrIdentityStoreRequired = errors.New("agent: IdentityStore is required")
	ErrAuthorityRequired     = errors.New("agent: authority decision is required for production identity lifecycle action")
)

// IdentityStore persists production signing identity material. Implementations
// must not log or publish PrivateKey; EventGraph records only receive public
// key references.
type IdentityStore interface {
	LoadIdentity(ctx context.Context, ref string) (PersistentIdentity, error)
	SaveIdentity(ctx context.Context, identity PersistentIdentity) error
}

// IdentityRecordStore is the narrow EventGraph v3.9 adapter boundary used by
// agent identity helpers. It accepts Tier 0 records without making agent own
// EventGraph schema or path-query behavior.
type IdentityRecordStore interface {
	AppendRecord(v39.Record) (v39.Record, error)
}

// IdentityAuthority contains the explicit v3.9 authority request/decision pair
// for a protected identity lifecycle action.
type IdentityAuthority struct {
	Request  v39.AuthorityRequest
	Decision v39.AuthorityDecision
}

// PersistentIdentity is local key-store state for a production agent identity.
type PersistentIdentity struct {
	Ref                string             `json:"ref"`
	ActorID            string             `json:"actor_id"`
	Name               string             `json:"name"`
	Mode               IdentityMode       `json:"mode"`
	Status             string             `json:"status"`
	Version            int                `json:"version"`
	PrivateKey         ed25519.PrivateKey `json:"private_key"`
	PublicKey          ed25519.PublicKey  `json:"public_key"`
	PreviousPublicKeys []string           `json:"previous_public_keys,omitempty"`
	AuthorityDecision  string             `json:"authority_decision,omitempty"`
	CreatedAt          time.Time          `json:"created_at"`
	UpdatedAt          time.Time          `json:"updated_at"`
}

// IdentityOperation configures a production identity lifecycle mutation.
type IdentityOperation struct {
	Store       IdentityStore
	RecordStore IdentityRecordStore
	Ref         string
	Name        string
	SigningKey  ed25519.PrivateKey
	Authority   *IdentityAuthority
	Reason      string
	CreatedBy   string
}

func resolvePersistentIdentity(ctx context.Context, cfg Config) (PersistentIdentity, error) {
	if cfg.PersistentIdentityRef == "" {
		return PersistentIdentity{}, ErrIdentityRefRequired
	}
	identity, err := cfg.IdentityStore.LoadIdentity(ctx, cfg.PersistentIdentityRef)
	if err == nil {
		if identity.Status == identityStatusRevoked {
			return PersistentIdentity{}, ErrIdentityRevoked
		}
		if err := validateStoredIdentity(cfg.Name, identity); err != nil {
			return PersistentIdentity{}, err
		}
		return identity, nil
	}
	if !errors.Is(err, ErrIdentityNotFound) {
		return PersistentIdentity{}, err
	}
	if !cfg.CreatePersistentIdentity {
		return PersistentIdentity{}, fmt.Errorf("%w: %s", ErrIdentityNotFound, cfg.PersistentIdentityRef)
	}

	op := IdentityOperation{
		Store:       cfg.IdentityStore,
		RecordStore: cfg.IdentityRecordStore,
		Ref:         cfg.PersistentIdentityRef,
		Name:        cfg.Name,
		SigningKey:  cfg.SigningKey,
		Authority:   cfg.IdentityAuthority,
		Reason:      "register persistent production agent identity",
	}
	return RegisterPersistentIdentity(ctx, op)
}

// RegisterPersistentIdentity creates and stores a production persistent
// identity after explicit authority approval.
func RegisterPersistentIdentity(ctx context.Context, op IdentityOperation) (PersistentIdentity, error) {
	if err := requireIdentityOperation(op, identityActionSpawnPersistent); err != nil {
		return PersistentIdentity{}, err
	}
	if _, err := op.Store.LoadIdentity(ctx, op.Ref); err == nil {
		return PersistentIdentity{}, fmt.Errorf("agent: persistent identity already exists: %s", op.Ref)
	} else if !errors.Is(err, ErrIdentityNotFound) {
		return PersistentIdentity{}, err
	}

	privateKey, mode, err := productionIdentityKey(op.Name, op.SigningKey)
	if err != nil {
		return PersistentIdentity{}, err
	}
	now := time.Now().UTC()
	publicKey := privateKey.Public().(ed25519.PublicKey)
	identity := PersistentIdentity{
		Ref:               op.Ref,
		ActorID:           actorIDForPublicKey(publicKey),
		Name:              op.Name,
		Mode:              mode,
		Status:            identityStatusActive,
		Version:           1,
		PrivateKey:        privateKey,
		PublicKey:         append(ed25519.PublicKey(nil), publicKey...),
		AuthorityDecision: op.Authority.Decision.ID,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := op.Store.SaveIdentity(ctx, identity); err != nil {
		return PersistentIdentity{}, err
	}
	if err := recordIdentityLifecycle(op, identity, "proposed", identityStatusActive, identityActionSpawnPersistent, "succeeded"); err != nil {
		return PersistentIdentity{}, err
	}
	return identity, nil
}

// RotatePersistentIdentity changes production key material while preserving the
// identity ref, version continuity, and audit trail.
func RotatePersistentIdentity(ctx context.Context, op IdentityOperation) (PersistentIdentity, error) {
	if err := requireIdentityOperation(op, identityActionRotateKey); err != nil {
		return PersistentIdentity{}, err
	}
	current, err := op.Store.LoadIdentity(ctx, op.Ref)
	if err != nil {
		return PersistentIdentity{}, err
	}
	if current.Status == identityStatusRevoked {
		return PersistentIdentity{}, ErrIdentityRevoked
	}
	privateKey, mode, err := productionIdentityKey(op.Name, op.SigningKey)
	if err != nil {
		return PersistentIdentity{}, err
	}
	publicKey := privateKey.Public().(ed25519.PublicKey)
	rotated := current
	rotated.Name = op.Name
	rotated.Mode = mode
	rotated.Version++
	rotated.PrivateKey = privateKey
	rotated.PublicKey = append(ed25519.PublicKey(nil), publicKey...)
	rotated.PreviousPublicKeys = append(append([]string(nil), current.PreviousPublicKeys...), publicKeyRef(current.PublicKey))
	rotated.AuthorityDecision = op.Authority.Decision.ID
	rotated.UpdatedAt = time.Now().UTC()
	if err := op.Store.SaveIdentity(ctx, rotated); err != nil {
		return PersistentIdentity{}, err
	}
	if err := recordIdentityLifecycle(op, rotated, identityStatusActive, identityStatusActive, identityActionRotateKey, "succeeded"); err != nil {
		return PersistentIdentity{}, err
	}
	return rotated, nil
}

// RevokePersistentIdentity marks a production identity unusable for future
// Agent construction and records the protected lifecycle action.
func RevokePersistentIdentity(ctx context.Context, op IdentityOperation) error {
	if err := requireIdentityOperation(op, identityActionRevoke); err != nil {
		return err
	}
	current, err := op.Store.LoadIdentity(ctx, op.Ref)
	if err != nil {
		return err
	}
	if current.Status == identityStatusRevoked {
		return ErrIdentityRevoked
	}
	revoked := current
	revoked.Status = identityStatusRevoked
	revoked.AuthorityDecision = op.Authority.Decision.ID
	revoked.UpdatedAt = time.Now().UTC()
	if err := op.Store.SaveIdentity(ctx, revoked); err != nil {
		return err
	}
	return recordIdentityLifecycle(op, revoked, identityStatusActive, identityStatusRevoked, identityActionRevoke, "succeeded")
}

func requireIdentityOperation(op IdentityOperation, action string) error {
	if op.Store == nil {
		return ErrIdentityStoreRequired
	}
	if op.RecordStore == nil {
		return fmt.Errorf("agent: IdentityRecordStore is required for %s", action)
	}
	if op.Ref == "" {
		return ErrIdentityRefRequired
	}
	if op.Name == "" {
		return fmt.Errorf("agent: identity name is required")
	}
	return requireAuthority(op.Authority, action, op.Ref)
}

func requireAuthority(authority *IdentityAuthority, action, target string) error {
	if authority == nil {
		return ErrAuthorityRequired
	}
	if authority.Request.Action != action {
		return fmt.Errorf("agent: authority request action %q does not permit %q", authority.Request.Action, action)
	}
	if authority.Request.TargetID != target {
		return fmt.Errorf("agent: authority request target %q does not permit %q", authority.Request.TargetID, target)
	}
	if authority.Decision.AuthorityRequestID != authority.Request.ID {
		return fmt.Errorf("agent: authority decision does not match request")
	}
	switch authority.Decision.Decision {
	case "Autonomous", "Notify":
	default:
		return fmt.Errorf("agent: authority decision %q does not allow %s", authority.Decision.Decision, action)
	}
	if authority.Decision.ExpiresAt != nil && time.Now().After(*authority.Decision.ExpiresAt) {
		return fmt.Errorf("agent: authority decision expired")
	}
	return nil
}

func productionIdentityKey(name string, supplied ed25519.PrivateKey) (ed25519.PrivateKey, IdentityMode, error) {
	if supplied != nil {
		if len(supplied) != ed25519.PrivateKeySize {
			return nil, "", fmt.Errorf("agent: SigningKey must be %d bytes", ed25519.PrivateKeySize)
		}
		if isPublicNameDerivedKey(name, supplied) {
			return nil, "", fmt.Errorf("agent: public-name-derived identity is blocked in production")
		}
		return append(ed25519.PrivateKey(nil), supplied...), IdentityModeExternallyManaged, nil
	}
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, "", fmt.Errorf("agent: generate signing key: %w", err)
	}
	return privateKey, IdentityModeGenerated, nil
}

func validateStoredIdentity(name string, identity PersistentIdentity) error {
	if identity.Status == "" {
		return fmt.Errorf("agent: persistent identity status is required")
	}
	if len(identity.PrivateKey) != ed25519.PrivateKeySize {
		return fmt.Errorf("agent: stored signing key must be %d bytes", ed25519.PrivateKeySize)
	}
	if isPublicNameDerivedKey(name, identity.PrivateKey) {
		return fmt.Errorf("agent: public-name-derived identity is blocked in production")
	}
	return nil
}

func recordIdentityLifecycle(op IdentityOperation, identity PersistentIdentity, fromState, toState, action, result string) error {
	now := time.Now().UTC()
	createdBy := op.CreatedBy
	if createdBy == "" {
		createdBy = op.Authority.Decision.DeciderActorID
	}
	if _, err := op.RecordStore.AppendRecord(&op.Authority.Request); err != nil {
		return fmt.Errorf("record authority request: %w", err)
	}
	if _, err := op.RecordStore.AppendRecord(&op.Authority.Decision); err != nil {
		return fmt.Errorf("record authority decision: %w", err)
	}
	status := identity.Status
	if _, err := op.RecordStore.AppendRecord(&v39.ActorIdentity{
		CommonNode: v39.CommonNode{
			ID:             recordID("ai", op.Ref, identity.Version),
			Type:           v39.TypeActorIdentity,
			CreatedAt:      now,
			CreatedBy:      createdBy,
			Status:         &status,
			Version:        identity.Version,
			IdempotencyKey: idempotencyKey("actor-identity", action, op.Ref, identity.Version),
			CorrelationID:  op.Authority.Request.CorrelationID,
			SourceRefs:     []string{action},
		},
		ActorID:      identity.ActorID,
		ActorType:    "agent",
		PublicKeyRef: stringPtr(publicKeyRef(identity.PublicKey)),
		IdentityMode: identityModeForRecord(identity.Mode),
	}); err != nil {
		return fmt.Errorf("record actor identity: %w", err)
	}
	if fromState != "" || toState != "" {
		if _, err := op.RecordStore.AppendRecord(&v39.LifecycleTransition{
			CommonNode: v39.CommonNode{
				ID:             recordID("lt", op.Ref, identity.Version),
				Type:           v39.TypeLifecycleTransition,
				CreatedAt:      now,
				CreatedBy:      createdBy,
				IdempotencyKey: idempotencyKey("lifecycle-transition", action, op.Ref, identity.Version),
				CorrelationID:  op.Authority.Request.CorrelationID,
				SourceRefs:     []string{action},
			},
			ActorID:             identity.ActorID,
			FromState:           fromState,
			ToState:             toState,
			Reason:              reasonOrDefault(op.Reason, action),
			AuthorityDecisionID: &op.Authority.Decision.ID,
		}); err != nil {
			return fmt.Errorf("record lifecycle transition: %w", err)
		}
	}
	if _, err := op.RecordStore.AppendRecord(&v39.ExecutionReceipt{
		CommonNode: v39.CommonNode{
			ID:             recordID("er", op.Ref, identity.Version),
			Type:           v39.TypeExecutionReceipt,
			CreatedAt:      now,
			CreatedBy:      createdBy,
			IdempotencyKey: idempotencyKey("execution-receipt", action, op.Ref, identity.Version),
			CorrelationID:  op.Authority.Request.CorrelationID,
			SourceRefs:     []string{action},
		},
		AuthorityDecisionID: op.Authority.Decision.ID,
		Action:              action,
		TargetID:            op.Ref,
		Result:              result,
		EvidenceRefs:        []string{publicKeyRef(identity.PublicKey)},
	}); err != nil {
		return fmt.Errorf("record execution receipt: %w", err)
	}
	return nil
}

func actorIDForPublicKey(publicKey ed25519.PublicKey) string {
	sum := sha256.Sum256(publicKey)
	return "actor_" + hex.EncodeToString(sum[:16])
}

func publicKeyRef(publicKey ed25519.PublicKey) string {
	sum := sha256.Sum256(publicKey)
	return "ed25519:" + hex.EncodeToString(sum[:])
}

func recordID(prefix, ref string, version int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d:%d", prefix, ref, version, time.Now().UnixNano())))
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(sum[:8]))
}

func idempotencyKey(prefix, action, ref string, version int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%s:%d", prefix, action, ref, version)))
	return fmt.Sprintf("agent:%s:%s", prefix, hex.EncodeToString(sum[:8]))
}

func identityModeForRecord(mode IdentityMode) string {
	switch mode {
	case IdentityModeExternallyManaged:
		return "externally_managed"
	case IdentityModeDeterministic:
		return "fixture"
	default:
		return "generated"
	}
}

func reasonOrDefault(reason, fallback string) string {
	if reason != "" {
		return reason
	}
	return fallback
}

func stringPtr(s string) *string { return &s }
