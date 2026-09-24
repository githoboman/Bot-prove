package hitl

import (
	"context"
	"strings"
	"testing"
)

func TestConfigFromEnv_DefaultsToNoop(t *testing.T) {
	t.Setenv("HITL_SINKS", "")
	t.Setenv("HITL_SLACK_WEBHOOK_URL", "")
	t.Setenv("HITL_TELEGRAM_BOT_TOKEN", "")
	t.Setenv("HITL_TELEGRAM_CHAT_ID", "")

	cfg := ConfigFromEnv()
	if len(cfg.Kinds) != 1 || cfg.Kinds[0] != "noop" {
		t.Fatalf("kinds = %v, want [noop]", cfg.Kinds)
	}
	sink, err := cfg.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, ok := sink.(NoopSink); !ok {
		t.Fatalf("sink type = %T, want NoopSink", sink)
	}
}

func TestConfigFromEnv_ParsesMultipleKinds(t *testing.T) {
	t.Setenv("HITL_SINKS", "slack, telegram")
	t.Setenv("HITL_SLACK_WEBHOOK_URL", "https://slack.test/hook")
	t.Setenv("HITL_TELEGRAM_BOT_TOKEN", "abc123")
	t.Setenv("HITL_TELEGRAM_CHAT_ID", "-1001")

	cfg := ConfigFromEnv()
	if len(cfg.Kinds) != 2 || cfg.Kinds[0] != "slack" || cfg.Kinds[1] != "telegram" {
		t.Fatalf("kinds = %v", cfg.Kinds)
	}
	if cfg.SlackWebhookURL != "https://slack.test/hook" {
		t.Errorf("slack url not read")
	}
	if cfg.TelegramBotToken != "abc123" {
		t.Errorf("tg token not read")
	}
	if cfg.TelegramChatID != -1001 {
		t.Errorf("tg chat id = %d", cfg.TelegramChatID)
	}

	sink, err := cfg.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	multi, ok := sink.(*MultiSink)
	if !ok {
		t.Fatalf("sink type = %T, want *MultiSink", sink)
	}
	if len(multi.Sinks) != 2 {
		t.Fatalf("MultiSink len = %d", len(multi.Sinks))
	}
	if _, ok := multi.Sinks[0].(*SlackSink); !ok {
		t.Errorf("sinks[0] = %T, want *SlackSink", multi.Sinks[0])
	}
	if _, ok := multi.Sinks[1].(*TelegramSink); !ok {
		t.Errorf("sinks[1] = %T, want *TelegramSink", multi.Sinks[1])
	}
}

func TestConfigFromEnv_SingleSlack(t *testing.T) {
	t.Setenv("HITL_SINKS", "slack")
	t.Setenv("HITL_SLACK_WEBHOOK_URL", "https://slack.test/hook")

	sink, err := ConfigFromEnv().Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, ok := sink.(*SlackSink); !ok {
		t.Fatalf("sink type = %T, want *SlackSink", sink)
	}
}

func TestConfig_UnknownKindErrors(t *testing.T) {
	cfg := SinkConfig{Kinds: []string{"pagerduty"}}
	if _, err := cfg.Build(); err == nil || !strings.Contains(err.Error(), "unknown sink kind") {
		t.Fatalf("expected unknown sink error, got %v", err)
	}
}

func TestConfig_MissingSlackURL(t *testing.T) {
	cfg := SinkConfig{Kinds: []string{"slack"}}
	if _, err := cfg.Build(); err == nil {
		t.Fatal("expected error on missing slack url")
	}
}

func TestConfig_MissingTelegramCreds(t *testing.T) {
	if _, err := (SinkConfig{Kinds: []string{"telegram"}, TelegramBotToken: "tok"}).Build(); err == nil {
		t.Fatal("expected error on missing chat id")
	}
	if _, err := (SinkConfig{Kinds: []string{"telegram"}, TelegramChatID: 42}).Build(); err == nil {
		t.Fatal("expected error on missing bot token")
	}
}

func TestNoopSink_Deliver(t *testing.T) {
	if err := (NoopSink{}).Deliver(context.Background(), newTestEvent(t)); err != nil {
		t.Fatalf("noop Deliver returned error: %v", err)
	}
}
