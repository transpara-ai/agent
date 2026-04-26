package agent

import (
	"fmt"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
)

// EmitHealthReport records a health.report event on the graph.
// Returns an error if the agent is retired or suspended.
func (a *Agent) EmitHealthReport(content event.HealthReportContent) error {
	if err := a.checkCanEmit(); err != nil {
		return fmt.Errorf("health report: %w", err)
	}
	_, err := a.recordAndTrack(event.EventTypeHealthReport.Value(), content)
	if err != nil {
		return fmt.Errorf("health report: %w", err)
	}
	return nil
}
