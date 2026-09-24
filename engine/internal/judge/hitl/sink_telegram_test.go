package hitl

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTelegramSink_NewValidatesInput(t *testing.T) {
	if _, err := NewTelegramSink("", 42); err == nil {
		t.Fatal("expected error on empty token")
	}
	if _, err := NewTelegramSink("tok", 0); err == nil {
		t.Fatal("expected error on zero chat id")
	}
	s, err := NewTelegramSink("tok", -100123)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.ChatID != -100123 || s.BotToken != "tok" {
		t.Fatal("fields not set")
	}
}

func TestTelegramSink_DeliverSendsRequestAndSucceedsOnOK(t *testing.T) {
	var gotPath string
	var gotPayload telegramSendMessageRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotPayload)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	sink := &TelegramSink{
		BotToken:   "bot-tok",
		ChatID:     -1001234567,
		APIBase:    srv.URL,
		HTTPClient: srv.Client(),
	}
	ev := newTestEvent(t)

	if err := sink.Deliver(context.Background(), ev); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if gotPath != "/botbot-tok/sendMessage" {
		t.Errorf("path=%q, want /botbot-tok/sendMessage", gotPath)
	}
	if gotPayload.ChatID != -1001234567 {
		t.Errorf("chatid=%d", gotPayload.ChatID)
	}
	if gotPayload.ParseMode != "MarkdownV2" {
		t.Errorf("parse_mode=%q", gotPayload.ParseMode)
	}
	if !strings.Contains(gotPayload.Text, "task\\-42") {
		t.Errorf("text missing escaped task id: %q", gotPayload.Text)
	}
	if !strings.Contains(gotPayload.Text, "abcdef0123456789") {
		t.Errorf("text missing digest: %q", gotPayload.Text)
	}
}

func TestTelegramSink_DeliverErrorsOnAPINotOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"chat not found"}`))
	}))
	defer srv.Close()

	sink := &TelegramSink{BotToken: "tok", ChatID: 42, APIBase: srv.URL, HTTPClient: srv.Client()}
	err := sink.Deliver(context.Background(), newTestEvent(t))
	if err == nil {
		t.Fatal("expected error on ok=false")
	}
	if !strings.Contains(err.Error(), "chat not found") {
		t.Errorf("error missing description: %v", err)
	}
}

func TestTelegramSink_DeliverErrorsOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("upstream busted"))
	}))
	defer srv.Close()

	sink := &TelegramSink{BotToken: "tok", ChatID: 42, APIBase: srv.URL, HTTPClient: srv.Client()}
	err := sink.Deliver(context.Background(), newTestEvent(t))
	if err == nil {
		t.Fatal("expected error on 500")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error missing status: %v", err)
	}
}

func TestTelegramSink_UnconfiguredErrors(t *testing.T) {
	sink := &TelegramSink{}
	err := sink.Deliver(context.Background(), newTestEvent(t))
	if err == nil {
		t.Fatal("expected error on unconfigured sink")
	}
}

func TestEscapeMarkdownV2(t *testing.T) {
	got := escapeMarkdownV2("hello_world*!")
	want := `hello\_world\*\!`
	if got != want {
		t.Errorf("escapeMarkdownV2 = %q, want %q", got, want)
	}
}
