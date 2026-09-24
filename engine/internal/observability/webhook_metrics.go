package observability

// Webhook-subsystem metrics — Prometheus counters + histograms that
// slot into the same Registry used by /metrics.
//
// The webhook worker calls into these on every enqueue, delivery
// attempt, success, failure, dead-letter and replay. Cardinality is
// bounded: `event` is one of the KnownWebhookEvents constants (a
// closed set defined in internal/api/webhooks.go); `status_class` is
// one of "2xx" / "4xx" / "5xx" / "network"; `outcome` is
// "delivered" / "retried" / "dead_lettered".
//
// Kept in the observability package (not internal/api) so
// unit-testing does not need to spin up a Server.

// WebhookMetrics groups every counter/histogram the webhook worker
// touches. Register once at Server startup; pass a *WebhookMetrics
// (or nil) into the webhook store — nil means metrics are disabled,
// which is what tests want.
type WebhookMetrics struct {
	// Enqueued counts events accepted onto the delivery queue,
	// partitioned by event kind. One increment per (subscription,
	// event) pairing that survives the subscribedTo filter.
	Enqueued *Counter

	// Attempts counts individual HTTP dispatch attempts, labeled by
	// event kind and outcome status class. Each retry is its own
	// increment.
	Attempts *Counter

	// Delivered counts terminal successes (any 2xx). At most one
	// increment per event, ever.
	Delivered *Counter

	// DeadLettered counts terminal failures — the event ran out of
	// retries. At most one increment per event, ever.
	DeadLettered *Counter

	// Replayed counts operator-initiated dead-letter replays. Same
	// event may be replayed several times; each call is one
	// increment.
	Replayed *Counter

	// AttemptDuration is the wall-clock time of a single HTTP
	// dispatch attempt (request build → response body drained).
	// Buckets are HTTP-latency-oriented (DefaultBuckets), labeled by
	// event kind + outcome status class.
	AttemptDuration *Histogram

	// QueueDepth is the current pending queue length. Sampled by the
	// worker on every tick — a gauge rather than a counter so it
	// reflects live pressure.
	QueueDepth *Gauge

	// DeadLetterDepth is the current dead-letter list length.
	// Sampled after every recordFailure that produces a dead letter,
	// and after every replay.
	DeadLetterDepth *Gauge
}

// NewWebhookMetrics registers the standard set on r under the given
// prefix (typically "cp_webhook").
func NewWebhookMetrics(r *Registry, prefix string) *WebhookMetrics {
	return &WebhookMetrics{
		Enqueued: r.NewCounter(
			prefix+"_enqueued_total",
			"Webhook events fanned out onto the delivery queue.",
			"event",
		),
		Attempts: r.NewCounter(
			prefix+"_attempts_total",
			"Individual HTTP dispatch attempts by outcome status class.",
			"event", "status_class",
		),
		Delivered: r.NewCounter(
			prefix+"_delivered_total",
			"Webhook events that reached a 2xx response before max attempts.",
			"event",
		),
		DeadLettered: r.NewCounter(
			prefix+"_dead_lettered_total",
			"Webhook events that exhausted their retry budget.",
			"event",
		),
		Replayed: r.NewCounter(
			prefix+"_replayed_total",
			"Operator-initiated dead-letter replay calls.",
			"event",
		),
		AttemptDuration: r.NewHistogram(
			prefix+"_attempt_duration_seconds",
			"Latency of one webhook HTTP dispatch attempt, in seconds.",
			nil,
			"event", "status_class",
		),
		QueueDepth: r.NewGauge(
			prefix+"_queue_depth",
			"Current webhook delivery queue depth (pending events).",
		),
		DeadLetterDepth: r.NewGauge(
			prefix+"_dead_letter_depth",
			"Current dead-letter list depth.",
		),
	}
}

// StatusClass turns an HTTP status code (or 0 for a network error)
// into a low-cardinality label value.
func StatusClass(code int) string {
	switch {
	case code == 0:
		return "network"
	case code >= 200 && code < 300:
		return "2xx"
	case code >= 300 && code < 400:
		return "3xx"
	case code >= 400 && code < 500:
		return "4xx"
	case code >= 500 && code < 600:
		return "5xx"
	default:
		return "other"
	}
}
