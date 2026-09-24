package hitl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// SlackSink delivers an escalation to a Slack Incoming Webhook.
//
// It intentionally posts a compact human-readable summary rather than the full
// evidence blob: the digest is included so a reviewer can retrieve the raw
// event from durable storage by hash. Do not embed provider secrets or raw
// prompts that may contain user-sensitive input; the summary sticks to
// task metadata and per-facet outcomes.
type SlackSink struct {
	// WebhookURL is the full Slack Incoming Webhook URL. Required.
	WebhookURL string
	// HTTPClient is used for delivery. If nil, a sensible default with a
	// short timeout is created.
	HTTPClient *http.Client
	// UsernameOverride, if set, overrides the webhook's default posting
	// username. Optional.
	UsernameOverride string
	// IconEmoji, if set, overrides the webhook's default icon. Optional.
	IconEmoji string
}

// NewSlackSink returns a SlackSink with a preconfigured HTTP client. It
// validates that url is non-empty.
func NewSlackSink(webhookURL string) (*SlackSink, error) {
	if webhookURL == "" {
		return nil, errors.New("hitl: slack webhook url is empty")
	}
	return &SlackSink{
		WebhookURL: webhookURL,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}, nil
}

// slackPayload is the minimal shape Slack Incoming Webhooks accept. We use the
// simple "text" API (no Block Kit) to keep the wire format stable across
// workspaces regardless of admin restrictions.
type slackPayload struct {
	Text      string `json:"text"`
	Username  string `json:"username,omitempty"`
	IconEmoji string `json:"icon_emoji,omitempty"`
}

// Deliver posts a compact summary of the event to Slack.
func (s *SlackSink) Deliver(ctx context.Context, ev EscalationEvent) error {
	if s.WebhookURL == "" {
		return errors.New("hitl: slack sink not configured")
	}
	client := s.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	payload := slackPayload{
		Text:      formatSlackText(ev),
		Username:  s.UsernameOverride,
		IconEmoji: s.IconEmoji,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("hitl: slack marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("hitl: slack build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("hitl: slack post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Slack returns short plaintext bodies on error; surface them.
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("hitl: slack status %d: %s", resp.StatusCode, string(snippet))
	}
	return nil
}

// formatSlackText produces a compact, mrkdwn-formatted summary suitable for
// Slack. The digest is included for auditors to correlate against archival
// storage.
func formatSlackText(ev EscalationEvent) string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "*HITL escalation* — severity `%s`\n", ev.Severity)
	fmt.Fprintf(&buf, "*task*: `%s`  *overall*: `%s`\n", ev.TaskID, ev.Overall)
	fmt.Fprintf(&buf, "*reason*: %s\n", ev.Reason)
	fmt.Fprintf(&buf, "*digest*: `%s`\n", ev.Digest)
	if len(ev.Facets) > 0 {
		buf.WriteString("*facets*:\n")
		for _, f := range ev.Facets {
			fmt.Fprintf(&buf, "• `%s` → `%s`", f.FacetID, f.Verdict)
			if f.Winner != "" {
				fmt.Fprintf(&buf, " (winner=`%s`, agree=%.2f, live=%d)", f.Winner, f.AgreementFraction, f.LiveCount)
			}
			buf.WriteString("\n")
		}
	}
	return buf.String()
}
