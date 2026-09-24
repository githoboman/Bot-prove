package hitl

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestEvent(t *testing.T) EscalationEvent {
	t.Helper()
	return EscalationEvent{
		Version:   "hitl.v1",
		Timestamp: time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC),
		TaskID:    "task-42",
		Overall:   "ABSTAIN",
		Severity:  SeverityMedium,
		Reason:    "human review required: 1 AGREE, 2 ABSTAIN, 0 DISAGREE facets — no dominant verdict",
		Digest:    "abcdef0123456789",
		Facets: []FacetSummary{
			{FacetID: "f1", Verdict: "AGREE", Winner: "yes", AgreementFraction: 0.75, LiveCount: 4},
			{FacetID: "f2", Verdict: "ABSTAIN"},
		},
	}
}

func TestSlackSink_NewValidatesURL(t *testing.T) {
	if _, err := NewSlackSink(""); err == nil {
		t.Fatal("expected error on empty webhook url")
	}
	s, err := NewSlackSink("https://example.test/hook")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.WebhookURL != "https://example.test/hook" {
		t.Fatalf("webhook not set")
	}
	if s.HTTPClient == nil {
		t.Fatal("default http client not initialised")
	}
}

func TestSlackSink_DeliverPostsJSONPayload(t *testing.T) {
	var gotPayload slackPayload
	var gotMethod, gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotPayload)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	sink := &SlackSink{WebhookURL: srv.URL, HTTPClient: srv.Client(), UsernameOverride: "cp-judge", IconEmoji: ":robot_face:"}
	ev := newTestEvent(t)

	if err := sink.Deliver(context.Background(), ev); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method=%s, want POST", gotMethod)
	}
	if gotContentType != "application/json" {
		t.Errorf("content-type=%s", gotContentType)
	}
	if gotPayload.Username != "cp-judge" {
		t.Errorf("username=%q", gotPayload.Username)
	}
	if gotPayload.IconEmoji != ":robot_face:" {
		t.Errorf("icon=%q", gotPayload.IconEmoji)
	}
	if !strings.Contains(gotPayload.Text, "task-42") {
		t.Errorf("text missing task id: %q", gotPayload.Text)
	}
	if !strings.Contains(gotPayload.Text, "abcdef0123456789") {
		t.Errorf("text missing digest: %q", gotPayload.Text)
	}
	if !strings.Contains(gotPayload.Text, "medium") {
		t.Errorf("text missing severity: %q", gotPayload.Text)
	}
}

func TestSlackSink_DeliverReturnsErrorOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("no_service"))
	}))
	defer srv.Close()

	sink := &SlackSink{WebhookURL: srv.URL, HTTPClient: srv.Client()}
	err := sink.Deliver(context.Background(), newTestEvent(t))
	if err == nil {
		t.Fatal("expected error on 403")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error missing status: %v", err)
	}
	if !strings.Contains(err.Error(), "no_service") {
		t.Errorf("error missing body snippet: %v", err)
	}
}

func TestSlackSink_DeliverEmptyWebhookErrors(t *testing.T) {
	sink := &SlackSink{}
	err := sink.Deliver(context.Background(), newTestEvent(t))
	if err == nil {
		t.Fatal("expected error on unconfigured sink")
	}
}
