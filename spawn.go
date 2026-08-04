package agent

import (
	"fmt"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

// EmitRoleProposed records a role proposal submitted for governance review.
// Returns an error if the agent is retired or suspended.
func (a *Agent) EmitRoleProposed(content event.RoleProposedContent) error {
	if err := a.checkCanEmit(); err != nil {
		return fmt.Errorf("role proposed: %w", err)
	}
	_, err := a.recordAndTrack(event.EventTypeRoleProposed.Value(), content)
	if err != nil {
		return fmt.Errorf("role proposed: %w", err)
	}
	return nil
}

// EmitRoleProposedCausedBy records a role proposal with exactly one explicit
// cross-actor cause. The ordinary EmitRoleProposed private-chain behavior is
// unchanged.
func (a *Agent) EmitRoleProposedCausedBy(content event.RoleProposedContent, cause types.EventID) error {
	if err := a.checkCanEmit(); err != nil {
		return fmt.Errorf("role proposed: %w", err)
	}
	_, err := a.recordAndTrackCausedBy(event.EventTypeRoleProposed.Value(), content, cause)
	if err != nil {
		return fmt.Errorf("role proposed: %w", err)
	}
	return nil
}

// EmitRoleApproved records the approval of a proposed role.
// Returns an error if the agent is retired or suspended.
func (a *Agent) EmitRoleApproved(content event.RoleApprovedContent) error {
	if err := a.checkCanEmit(); err != nil {
		return fmt.Errorf("role approved: %w", err)
	}
	_, err := a.recordAndTrack(event.EventTypeRoleApproved.Value(), content)
	if err != nil {
		return fmt.Errorf("role approved: %w", err)
	}
	return nil
}

// EmitRoleApprovedCausedBy records a role approval with exactly one explicit
// proposal cause.
func (a *Agent) EmitRoleApprovedCausedBy(content event.RoleApprovedContent, cause types.EventID) error {
	if err := a.checkCanEmit(); err != nil {
		return fmt.Errorf("role approved: %w", err)
	}
	_, err := a.recordAndTrackCausedBy(event.EventTypeRoleApproved.Value(), content, cause)
	if err != nil {
		return fmt.Errorf("role approved: %w", err)
	}
	return nil
}

// EmitRoleRejected records the rejection of a proposed role.
// Returns an error if the agent is retired or suspended.
func (a *Agent) EmitRoleRejected(content event.RoleRejectedContent) error {
	if err := a.checkCanEmit(); err != nil {
		return fmt.Errorf("role rejected: %w", err)
	}
	_, err := a.recordAndTrack(event.EventTypeRoleRejected.Value(), content)
	if err != nil {
		return fmt.Errorf("role rejected: %w", err)
	}
	return nil
}

// EmitRoleRejectedCausedBy records a role rejection with exactly one explicit
// proposal cause.
func (a *Agent) EmitRoleRejectedCausedBy(content event.RoleRejectedContent, cause types.EventID) error {
	if err := a.checkCanEmit(); err != nil {
		return fmt.Errorf("role rejected: %w", err)
	}
	_, err := a.recordAndTrackCausedBy(event.EventTypeRoleRejected.Value(), content, cause)
	if err != nil {
		return fmt.Errorf("role rejected: %w", err)
	}
	return nil
}
