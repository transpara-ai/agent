package agent

import (
	"fmt"

	"github.com/lovyou-ai/eventgraph/go/pkg/event"
)

// EmitSiteOpReceived records the anchor event for a site op.
// Called synchronously from the hive webhook handler before responding.
// Returns an error if the agent is retired or suspended.
func (a *Agent) EmitSiteOpReceived(content event.SiteOpReceivedContent) error {
	if err := a.checkCanEmit(); err != nil {
		return fmt.Errorf("site op received: %w", err)
	}
	_, err := a.recordAndTrack(event.EventTypeSiteOpReceived.Value(), content)
	if err != nil {
		return fmt.Errorf("site op received: %w", err)
	}
	return nil
}

// EmitSiteOpTranslated records successful translation of an anchored op
// into a hive bus event.
// Returns an error if the agent is retired or suspended.
func (a *Agent) EmitSiteOpTranslated(content event.SiteOpTranslatedContent) error {
	if err := a.checkCanEmit(); err != nil {
		return fmt.Errorf("site op translated: %w", err)
	}
	_, err := a.recordAndTrack(event.EventTypeSiteOpTranslated.Value(), content)
	if err != nil {
		return fmt.Errorf("site op translated: %w", err)
	}
	return nil
}

// EmitSiteOpRejected records translation failure.
// Returns an error if the agent is retired or suspended.
func (a *Agent) EmitSiteOpRejected(content event.SiteOpRejectedContent) error {
	if err := a.checkCanEmit(); err != nil {
		return fmt.Errorf("site op rejected: %w", err)
	}
	_, err := a.recordAndTrack(event.EventTypeSiteOpRejected.Value(), content)
	if err != nil {
		return fmt.Errorf("site op rejected: %w", err)
	}
	return nil
}

// EmitSiteOpMirrored records successful mirror POST to site.
// Returns an error if the agent is retired or suspended.
func (a *Agent) EmitSiteOpMirrored(content event.SiteOpMirroredContent) error {
	if err := a.checkCanEmit(); err != nil {
		return fmt.Errorf("site op mirrored: %w", err)
	}
	_, err := a.recordAndTrack(event.EventTypeSiteOpMirrored.Value(), content)
	if err != nil {
		return fmt.Errorf("site op mirrored: %w", err)
	}
	return nil
}
