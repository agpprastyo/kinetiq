package alerting

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type DispatchPayload struct {
	RuleID         string `json:"rule_id,omitempty"`
	RuleName       string `json:"rule_name,omitempty"`
	ProjectID      string `json:"project_id,omitempty"`
	EventName      string `json:"event_name,omitempty"`
	Condition      string `json:"condition,omitempty"`
	CurrentCount   int64  `json:"current_count,omitempty"`
	ThresholdCount int64  `json:"threshold_count,omitempty"`
	WindowMinutes  int    `json:"window_minutes,omitempty"`
	TriggeredAt    string `json:"triggered_at,omitempty"`
	IsTest         bool   `json:"is_test,omitempty"`
	IsRecovery     bool   `json:"is_recovery,omitempty"`
}

type Dispatcher struct {
	client *http.Client
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		client: &http.Client{Timeout: 6 * time.Second},
	}
}

func (d *Dispatcher) Dispatch(ctx context.Context, webhookURL, secret string, payload []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Kinetiq-Alert-Engine/1.0")

	if secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(payload)
		signature := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Kinetiq-Signature", signature)
		req.Header.Set("X-Kinetiq-Signature", signature)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook responded with non-2xx status: %d", resp.StatusCode)
	}
	return nil
}

func (d *Dispatcher) DispatchRule(ctx context.Context, rule AlertRule, payload DispatchPayload) error {
	rawURL := strings.TrimSpace(rule.WebhookURL)
	if rawURL == "" {
		return fmt.Errorf("empty destination webhook URL")
	}

	// Dry run bypass for documentation/dummy URLs
	if strings.Contains(rawURL, "example.com") || strings.Contains(rawURL, "localhost:0") {
		return nil
	}

	switch strings.ToLower(rule.ChannelType) {
	case "slack":
		return d.dispatchSlack(ctx, rawURL, payload)
	case "discord":
		return d.dispatchDiscord(ctx, rawURL, payload)
	case "telegram":
		return d.dispatchTelegram(ctx, rawURL, rule.TelegramChatID, payload)
	default:
		body, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		return d.Dispatch(ctx, rawURL, rule.WebhookSecret, body)
	}
}

func (d *Dispatcher) dispatchSlack(ctx context.Context, webhookURL string, payload DispatchPayload) error {
	tag := "🚨 *[Kinetiq Alert Triggered]*"
	if payload.IsTest {
		tag = "🔔 *[Kinetiq Test Alert]*"
	} else if payload.IsRecovery {
		tag = "🟢 *[Kinetiq Alert Recovered]*"
	}

	slackBody := map[string]interface{}{
		"text": fmt.Sprintf("%s *%s*", tag, payload.RuleName),
		"blocks": []map[string]interface{}{
			{
				"type": "section",
				"text": map[string]string{
					"type": "mrkdwn",
					"text": fmt.Sprintf("%s *%s*\nEvent: `%s` | Threshold: `%d` in %dm | Current Count: *%d*",
						tag, payload.RuleName, payload.EventName, payload.ThresholdCount, payload.WindowMinutes, payload.CurrentCount),
				},
			},
		},
	}

	body, err := json.Marshal(slackBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("slack webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func (d *Dispatcher) dispatchDiscord(ctx context.Context, webhookURL string, payload DispatchPayload) error {
	title := "🚨 Kinetiq Alert Triggered"
	color := 16724530 // Red
	if payload.IsTest {
		title = "🔔 Kinetiq Test Notification"
		color = 1091561 // Emerald green
	} else if payload.IsRecovery {
		title = "🟢 Kinetiq Alert Recovered"
		color = 5763719 // Green
	}

	discordBody := map[string]interface{}{
		"embeds": []map[string]interface{}{
			{
				"title":       title,
				"description": fmt.Sprintf("**Rule:** %s\n**Event:** `%s`\n**Condition:** `%s`\n**Current Count:** `%d` (Threshold: %d in %dm)", payload.RuleName, payload.EventName, payload.Condition, payload.CurrentCount, payload.ThresholdCount, payload.WindowMinutes),
				"color":       color,
				"timestamp":   payload.TriggeredAt,
			},
		},
	}

	body, err := json.Marshal(discordBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("discord webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func (d *Dispatcher) dispatchTelegram(ctx context.Context, apiURLOrToken, chatID string, payload DispatchPayload) error {
	targetURL := apiURLOrToken
	if !strings.HasPrefix(targetURL, "http") {
		targetURL = fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", apiURLOrToken)
	}

	title := "🚨 *Kinetiq Alert Triggered*"
	if payload.IsTest {
		title = "🔔 *Kinetiq Test Alert*"
	} else if payload.IsRecovery {
		title = "🟢 *Kinetiq Alert Recovered*"
	}

	text := fmt.Sprintf("%s\n\n*Rule:* %s\n*Event:* `%s`\n*Condition:* `%s`\n*Current Volume:* `%d` (Threshold: %d in %dm)\n*Triggered:* `%s`",
		title, payload.RuleName, payload.EventName, payload.Condition, payload.CurrentCount, payload.ThresholdCount, payload.WindowMinutes, payload.TriggeredAt)

	tgBody := map[string]interface{}{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "Markdown",
	}

	body, err := json.Marshal(tgBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram returned HTTP %d", resp.StatusCode)
	}
	return nil
}
