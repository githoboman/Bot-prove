package hitl

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// SinkConfig is the declarative shape used to construct sinks from
// environment or a config file, without touching HTTP internals.
//
// Multiple sinks are combined into a MultiSink automatically. An empty
// config yields a NoopSink so the engine boots cleanly even without any
// HITL integration configured.
type SinkConfig struct {
	// Kinds is the comma-separated list of enabled sinks. Supported values:
	// "noop", "slack", "telegram". Order is preserved and drives the
	// MultiSink fan-out order.
	Kinds []string

	// Slack settings — used when "slack" is in Kinds.
	SlackWebhookURL string

	// Telegram settings — used when "telegram" is in Kinds.
	TelegramBotToken string
	TelegramChatID   int64
}

// ConfigFromEnv reads the HITL sink configuration from environment variables.
//
// Variables:
//
//	HITL_SINKS              comma-separated: e.g. "noop", "slack", "telegram",
//	                        "slack,telegram". Empty ⇒ "noop".
//	HITL_SLACK_WEBHOOK_URL  required when "slack" is in HITL_SINKS.
//	HITL_TELEGRAM_BOT_TOKEN required when "telegram" is in HITL_SINKS.
//	HITL_TELEGRAM_CHAT_ID   required when "telegram" is in HITL_SINKS.
//
// The function returns a populated SinkConfig; it does not validate that
// required fields are present — Build enforces that.
func ConfigFromEnv() SinkConfig {
	kindsRaw := strings.TrimSpace(os.Getenv("HITL_SINKS"))
	if kindsRaw == "" {
		kindsRaw = "noop"
	}
	parts := strings.Split(kindsRaw, ",")
	kinds := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(strings.ToLower(p))
		if p == "" {
			continue
		}
		kinds = append(kinds, p)
	}

	chatID, _ := strconv.ParseInt(strings.TrimSpace(os.Getenv("HITL_TELEGRAM_CHAT_ID")), 10, 64)

	return SinkConfig{
		Kinds:            kinds,
		SlackWebhookURL:  strings.TrimSpace(os.Getenv("HITL_SLACK_WEBHOOK_URL")),
		TelegramBotToken: strings.TrimSpace(os.Getenv("HITL_TELEGRAM_BOT_TOKEN")),
		TelegramChatID:   chatID,
	}
}

// Build constructs a Sink from the config. Rules:
//
//   - No kinds or only "noop" ⇒ NoopSink.
//   - Multiple kinds ⇒ MultiSink of each individually-built sink.
//   - Unknown kind ⇒ error.
//   - Missing required setting for a declared kind ⇒ error.
func (c SinkConfig) Build() (Sink, error) {
	if len(c.Kinds) == 0 {
		return NoopSink{}, nil
	}
	if len(c.Kinds) == 1 && c.Kinds[0] == "noop" {
		return NoopSink{}, nil
	}

	sinks := make([]Sink, 0, len(c.Kinds))
	for _, k := range c.Kinds {
		switch k {
		case "noop":
			sinks = append(sinks, NoopSink{})
		case "slack":
			s, err := NewSlackSink(c.SlackWebhookURL)
			if err != nil {
				return nil, fmt.Errorf("hitl config: slack: %w", err)
			}
			sinks = append(sinks, s)
		case "telegram":
			s, err := NewTelegramSink(c.TelegramBotToken, c.TelegramChatID)
			if err != nil {
				return nil, fmt.Errorf("hitl config: telegram: %w", err)
			}
			sinks = append(sinks, s)
		default:
			return nil, fmt.Errorf("hitl config: unknown sink kind %q", k)
		}
	}

	if len(sinks) == 1 {
		return sinks[0], nil
	}
	return &MultiSink{Sinks: sinks}, nil
}
