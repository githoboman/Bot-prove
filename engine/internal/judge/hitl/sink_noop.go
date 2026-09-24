package hitl

import "context"

// NoopSink discards every event. Used when HITL is disabled by configuration
// or when the judge is running in an environment (CI, unit tests) where
// external side-effects are undesirable.
//
// The zero value is ready to use.
type NoopSink struct{}

// Deliver always returns nil.
func (NoopSink) Deliver(_ context.Context, _ EscalationEvent) error {
	return nil
}
