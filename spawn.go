package agent

import (
	"fmt"

	"github.com/lovyou-ai/eventgraph/go/pkg/event"
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
