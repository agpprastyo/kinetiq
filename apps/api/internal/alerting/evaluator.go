package alerting

import (
	"time"
)

type AlertRule struct {
	ID              string     `json:"id"`
	ProjectID       string     `json:"project_id"`
	Name            string     `json:"name"`
	EventName       string     `json:"event_name"`
	Condition       string     `json:"condition"` // spike_gt, drop_lt
	ThresholdCount  int64      `json:"threshold_count"`
	WindowMinutes   int        `json:"window_minutes"`
	WebhookURL      string     `json:"webhook_url"`
	WebhookSecret   string     `json:"webhook_secret"`
	ChannelType     string     `json:"channel_type"` // slack, discord, telegram, webhook
	TelegramChatID  string     `json:"telegram_chat_id"`
	IsActive        bool       `json:"is_active"`
	CooldownMinutes int        `json:"cooldown_minutes"`
	LastTriggeredAt *time.Time `json:"last_triggered_at,omitempty"`
	SnoozedUntil    *time.Time `json:"snoozed_until,omitempty"`
	Status          string     `json:"status"` // healthy, firing, snoozed
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// EvaluateAlert checks if current event count breaches rule threshold
func EvaluateAlert(rule AlertRule, currentVolume int64) bool {
	switch rule.Condition {
	case "spike_gt":
		return currentVolume > rule.ThresholdCount
	case "drop_lt":
		return currentVolume < rule.ThresholdCount
	default:
		return false
	}
}
