package agent

import (
	"fmt"

	"github.com/lovyou-ai/eventgraph/go/pkg/event"
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

// EmitAgentVitalReported records a single agent.vital.reported event on the
// graph. Emitted by SysMon once per AgentVital per health-report cycle; the
// HealthReportCycleID on content correlates each vital back to the umbrella
// health.report event in the same cycle.
func (a *Agent) EmitAgentVitalReported(content event.AgentVitalReportedContent) error {
	if err := a.checkCanEmit(); err != nil {
		return fmt.Errorf("agent vital reported: %w", err)
	}
	_, err := a.recordAndTrack(event.EventTypeAgentVitalReported.Value(), content)
	if err != nil {
		return fmt.Errorf("agent vital reported: %w", err)
	}
	return nil
}
