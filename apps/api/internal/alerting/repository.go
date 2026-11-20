package alerting

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrAlertRuleNotFound = errors.New("alert rule not found")
)

type Repository interface {
	CreateRule(ctx context.Context, rule AlertRule) (*AlertRule, error)
	GetRule(ctx context.Context, ruleID uuid.UUID) (*AlertRule, error)
	ListRules(ctx context.Context, projectID uuid.UUID) ([]AlertRule, error)
	ListActiveRulesForProject(ctx context.Context, projectID uuid.UUID) ([]AlertRule, error)
	UpdateRule(ctx context.Context, rule AlertRule) (*AlertRule, error)
	DeleteRule(ctx context.Context, ruleID uuid.UUID) error
	UpdateLastTriggered(ctx context.Context, ruleID uuid.UUID, triggeredAt time.Time) error
	UpdateRuleStatus(ctx context.Context, ruleID uuid.UUID, status string) error
	SnoozeRule(ctx context.Context, ruleID uuid.UUID, snoozedUntil *time.Time) (*AlertRule, error)
}

type SQLRepository struct {
	pool *pgxpool.Pool
}

func NewSQLRepository(pool *pgxpool.Pool) *SQLRepository {
	return &SQLRepository{pool: pool}
}

func (r *SQLRepository) CreateRule(ctx context.Context, rule AlertRule) (*AlertRule, error) {
	ruleID := uuid.New()
	if rule.ID != "" {
		if parsed, err := uuid.Parse(rule.ID); err == nil {
			ruleID = parsed
		}
	}

	channelType := strings.ToLower(rule.ChannelType)
	if channelType == "" {
		channelType = "webhook"
	}
	cooldown := rule.CooldownMinutes
	if cooldown <= 0 {
		cooldown = 15
	}
	window := rule.WindowMinutes
	if window <= 0 {
		window = 5
	}

	var created AlertRule
	status := rule.Status
	if status == "" {
		status = "healthy"
	}
	err := r.pool.QueryRow(ctx, `
		INSERT INTO alert_rules (
			id, project_id, name, event_name, condition,
			threshold_count, window_minutes, webhook_url, webhook_secret,
			channel_type, telegram_chat_id, is_active, cooldown_minutes,
			snoozed_until, status, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9,
			$10, $11, $12, $13,
			$14, $15, NOW(), NOW()
		)
		RETURNING id, project_id, name, event_name, condition,
		          threshold_count, window_minutes, webhook_url, webhook_secret,
		          channel_type, telegram_chat_id, is_active, cooldown_minutes,
		          last_triggered_at, snoozed_until, status, created_at, updated_at
	`, ruleID, rule.ProjectID, rule.Name, rule.EventName, rule.Condition,
		rule.ThresholdCount, window, rule.WebhookURL, rule.WebhookSecret,
		channelType, rule.TelegramChatID, rule.IsActive, cooldown,
		rule.SnoozedUntil, status).Scan(
		&created.ID,
		&created.ProjectID,
		&created.Name,
		&created.EventName,
		&created.Condition,
		&created.ThresholdCount,
		&created.WindowMinutes,
		&created.WebhookURL,
		&created.WebhookSecret,
		&created.ChannelType,
		&created.TelegramChatID,
		&created.IsActive,
		&created.CooldownMinutes,
		&created.LastTriggeredAt,
		&created.SnoozedUntil,
		&created.Status,
		&created.CreatedAt,
		&created.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &created, nil
}

func (r *SQLRepository) GetRule(ctx context.Context, ruleID uuid.UUID) (*AlertRule, error) {
	var rule AlertRule
	err := r.pool.QueryRow(ctx, `
		SELECT id, project_id, name, event_name, condition,
		       threshold_count, window_minutes, webhook_url, webhook_secret,
		       channel_type, telegram_chat_id, is_active, cooldown_minutes,
		       last_triggered_at, snoozed_until, status, created_at, updated_at
		FROM alert_rules
		WHERE id = $1
	`, ruleID).Scan(
		&rule.ID,
		&rule.ProjectID,
		&rule.Name,
		&rule.EventName,
		&rule.Condition,
		&rule.ThresholdCount,
		&rule.WindowMinutes,
		&rule.WebhookURL,
		&rule.WebhookSecret,
		&rule.ChannelType,
		&rule.TelegramChatID,
		&rule.IsActive,
		&rule.CooldownMinutes,
		&rule.LastTriggeredAt,
		&rule.SnoozedUntil,
		&rule.Status,
		&rule.CreatedAt,
		&rule.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAlertRuleNotFound
		}
		return nil, err
	}
	return &rule, nil
}

func (r *SQLRepository) ListRules(ctx context.Context, projectID uuid.UUID) ([]AlertRule, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, project_id, name, event_name, condition,
		       threshold_count, window_minutes, webhook_url, webhook_secret,
		       channel_type, telegram_chat_id, is_active, cooldown_minutes,
		       last_triggered_at, snoozed_until, status, created_at, updated_at
		FROM alert_rules
		WHERE project_id = $1
		ORDER BY created_at DESC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rules := make([]AlertRule, 0)
	for rows.Next() {
		var rule AlertRule
		if err := rows.Scan(
			&rule.ID,
			&rule.ProjectID,
			&rule.Name,
			&rule.EventName,
			&rule.Condition,
			&rule.ThresholdCount,
			&rule.WindowMinutes,
			&rule.WebhookURL,
			&rule.WebhookSecret,
			&rule.ChannelType,
			&rule.TelegramChatID,
			&rule.IsActive,
			&rule.CooldownMinutes,
			&rule.LastTriggeredAt,
			&rule.SnoozedUntil,
			&rule.Status,
			&rule.CreatedAt,
			&rule.UpdatedAt,
		); err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

func (r *SQLRepository) ListActiveRulesForProject(ctx context.Context, projectID uuid.UUID) ([]AlertRule, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, project_id, name, event_name, condition,
		       threshold_count, window_minutes, webhook_url, webhook_secret,
		       channel_type, telegram_chat_id, is_active, cooldown_minutes,
		       last_triggered_at, snoozed_until, status, created_at, updated_at
		FROM alert_rules
		WHERE project_id = $1 AND is_active = true
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rules := make([]AlertRule, 0)
	for rows.Next() {
		var rule AlertRule
		if err := rows.Scan(
			&rule.ID,
			&rule.ProjectID,
			&rule.Name,
			&rule.EventName,
			&rule.Condition,
			&rule.ThresholdCount,
			&rule.WindowMinutes,
			&rule.WebhookURL,
			&rule.WebhookSecret,
			&rule.ChannelType,
			&rule.TelegramChatID,
			&rule.IsActive,
			&rule.CooldownMinutes,
			&rule.LastTriggeredAt,
			&rule.SnoozedUntil,
			&rule.Status,
			&rule.CreatedAt,
			&rule.UpdatedAt,
		); err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

func (r *SQLRepository) UpdateRule(ctx context.Context, rule AlertRule) (*AlertRule, error) {
	ruleUUID, err := uuid.Parse(rule.ID)
	if err != nil {
		return nil, err
	}

	channelType := strings.ToLower(rule.ChannelType)
	if channelType == "" {
		channelType = "webhook"
	}
	cooldown := rule.CooldownMinutes
	if cooldown <= 0 {
		cooldown = 15
	}
	window := rule.WindowMinutes
	if window <= 0 {
		window = 5
	}

	var updated AlertRule
	err = r.pool.QueryRow(ctx, `
		UPDATE alert_rules
		SET name = $2,
		    event_name = $3,
		    condition = $4,
		    threshold_count = $5,
		    window_minutes = $6,
		    webhook_url = $7,
		    webhook_secret = $8,
		    channel_type = $9,
		    telegram_chat_id = $10,
		    is_active = $11,
		    cooldown_minutes = $12,
		    updated_at = NOW()
		WHERE id = $1
		RETURNING id, project_id, name, event_name, condition,
		          threshold_count, window_minutes, webhook_url, webhook_secret,
		          channel_type, telegram_chat_id, is_active, cooldown_minutes,
		          last_triggered_at, snoozed_until, status, created_at, updated_at
	`, ruleUUID, rule.Name, rule.EventName, rule.Condition,
		rule.ThresholdCount, window, rule.WebhookURL, rule.WebhookSecret,
		channelType, rule.TelegramChatID, rule.IsActive, cooldown).Scan(
		&updated.ID,
		&updated.ProjectID,
		&updated.Name,
		&updated.EventName,
		&updated.Condition,
		&updated.ThresholdCount,
		&updated.WindowMinutes,
		&updated.WebhookURL,
		&updated.WebhookSecret,
		&updated.ChannelType,
		&updated.TelegramChatID,
		&updated.IsActive,
		&updated.CooldownMinutes,
		&updated.LastTriggeredAt,
		&updated.SnoozedUntil,
		&updated.Status,
		&updated.CreatedAt,
		&updated.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAlertRuleNotFound
		}
		return nil, err
	}
	return &updated, nil
}

func (r *SQLRepository) DeleteRule(ctx context.Context, ruleID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM alert_rules WHERE id = $1`, ruleID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrAlertRuleNotFound
	}
	return nil
}

func (r *SQLRepository) UpdateLastTriggered(ctx context.Context, ruleID uuid.UUID, triggeredAt time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE alert_rules
		SET last_triggered_at = $2,
		    status = 'firing'
		WHERE id = $1
	`, ruleID, triggeredAt)
	return err
}

func (r *SQLRepository) UpdateRuleStatus(ctx context.Context, ruleID uuid.UUID, status string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE alert_rules
		SET status = $2
		WHERE id = $1
	`, ruleID, status)
	return err
}

func (r *SQLRepository) SnoozeRule(ctx context.Context, ruleID uuid.UUID, snoozedUntil *time.Time) (*AlertRule, error) {
	status := "snoozed"
	if snoozedUntil == nil {
		status = "healthy"
	}
	var updated AlertRule
	err := r.pool.QueryRow(ctx, `
		UPDATE alert_rules
		SET snoozed_until = $2,
		    status = $3,
		    updated_at = NOW()
		WHERE id = $1
		RETURNING id, project_id, name, event_name, condition,
		          threshold_count, window_minutes, webhook_url, webhook_secret,
		          channel_type, telegram_chat_id, is_active, cooldown_minutes,
		          last_triggered_at, snoozed_until, status, created_at, updated_at
	`, ruleID, snoozedUntil, status).Scan(
		&updated.ID,
		&updated.ProjectID,
		&updated.Name,
		&updated.EventName,
		&updated.Condition,
		&updated.ThresholdCount,
		&updated.WindowMinutes,
		&updated.WebhookURL,
		&updated.WebhookSecret,
		&updated.ChannelType,
		&updated.TelegramChatID,
		&updated.IsActive,
		&updated.CooldownMinutes,
		&updated.LastTriggeredAt,
		&updated.SnoozedUntil,
		&updated.Status,
		&updated.CreatedAt,
		&updated.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAlertRuleNotFound
		}
		return nil, err
	}
	return &updated, nil
}
