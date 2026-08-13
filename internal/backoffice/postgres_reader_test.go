package backoffice

import (
	"testing"
	"time"
)

func TestDeriveCodeActivityDoesNotConfuseMissingTelemetryWithIdle(t *testing.T) {
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)

	activity := deriveCodeActivity(now, 0, nil, nil)

	if activity.State != CodeActivityUnobserved ||
		activity.Reason != ReasonNoConnectedObserver ||
		activity.ObserverConnected {
		t.Fatalf("activity = %#v", activity)
	}
}

func TestDeriveCodeActivityUsesFreshObservedDiff(t *testing.T) {
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	eventType := "pact.workspace.diff_updated.v1"
	observedAt := now.Add(-5 * time.Second)

	activity := deriveCodeActivity(now, 1, &eventType, &observedAt)

	if activity.State != CodeActivityEditing ||
		activity.Reason != ReasonFreshWorkspaceDiff ||
		!activity.ObserverConnected {
		t.Fatalf("activity = %#v", activity)
	}
}

func TestDeriveCodeActivityKeepsRecentEvidenceWithoutObserver(t *testing.T) {
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	eventType := "pact.git.external_change_detected.v1"
	observedAt := now.Add(-2 * time.Minute)

	activity := deriveCodeActivity(now, 0, &eventType, &observedAt)

	if activity.State != CodeActivityRecent ||
		activity.Reason != ReasonRecentExternalChange ||
		activity.ObserverConnected {
		t.Fatalf("activity = %#v", activity)
	}
}

func TestDeriveCodeActivityUsesIdleOnlyForCapableObserver(t *testing.T) {
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)

	activity := deriveCodeActivity(now, 1, nil, nil)

	if activity.State != CodeActivityIdle ||
		activity.Reason != ReasonObserverWithoutChange ||
		activity.ObserverCount != 1 ||
		activity.ObserverFreshSecs != 30 {
		t.Fatalf("activity = %#v", activity)
	}
}
