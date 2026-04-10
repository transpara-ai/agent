package agent

import (
	"fmt"

	"github.com/lovyou-ai/eventgraph/go/pkg/event"
)

// EmitHeartbeat records a hive.agent.heartbeat event on the graph.
// Returns an error if the agent is retired or suspended.
func (a *Agent) EmitHeartbeat(content event.EventContent) error {
	if err := a.checkCanEmit(); err != nil {
		return fmt.Errorf("heartbeat: %w", err)
	}
	_, err := a.recordAndTrack("hive.agent.heartbeat", content)
	if err != nil {
		return fmt.Errorf("heartbeat: %w", err)
	}
	return nil
}
