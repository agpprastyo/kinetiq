package alerting

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type Service interface {
	TrackEventAndEvaluate(ctx context.Context, projectID uuid.UUID, eventName string) error
	CreateRule(ctx context.Context, rule AlertRule) (*AlertRule, error)
	GetRule(ctx context.Context, ruleID uuid.UUID) (*AlertRule, error)
	ListRules(ctx context.Context, projectID uuid.UUID) ([]AlertRule, error)
	UpdateRule(ctx context.Context, rule AlertRule) (*AlertRule, error)
	DeleteRule(ctx context.Context, ruleID uuid.UUID) error
	TestRule(ctx context.Context, ruleID uuid.UUID) error
	SnoozeRule(ctx context.Context, ruleID uuid.UUID, durationHours int) (*AlertRule, error)
	EvaluateAndTrigger(ctx context.Context, rule AlertRule, currentCount int64, now time.Time)
}

type AlertingService struct {
	repo       Repository
	dispatcher *Dispatcher
	redisCli   *redis.Client
	logger     *slog.Logger
}

func NewService(repo Repository, dispatcher *Dispatcher, redisCli *redis.Client, logger *slog.Logger) *AlertingService {
	return &AlertingService{
		repo:       repo,
		dispatcher: dispatcher,
		redisCli:   redisCli,
		logger:     logger,
	}
}

func (s *AlertingService) CreateRule(ctx context.Context, rule AlertRule) (*AlertRule, error) {
	return s.repo.CreateRule(ctx, rule)
}

func (s *AlertingService) GetRule(ctx context.Context, ruleID uuid.UUID) (*AlertRule, error) {
	return s.repo.GetRule(ctx, ruleID)
}

func (s *AlertingService) ListRules(ctx context.Context, projectID uuid.UUID) ([]AlertRule, error) {
	return s.repo.ListRules(ctx, projectID)
}

func (s *AlertingService) UpdateRule(ctx context.Context, rule AlertRule) (*AlertRule, error) {
	return s.repo.UpdateRule(ctx, rule)
}

func (s *AlertingService) DeleteRule(ctx context.Context, ruleID uuid.UUID) error {
	return s.repo.DeleteRule(ctx, ruleID)
}

func (s *AlertingService) TestRule(ctx context.Context, ruleID uuid.UUID) error {
	rule, err := s.repo.GetRule(ctx, ruleID)
	if err != nil {
		return err
	}

	payload := DispatchPayload{
		RuleID:         rule.ID,
		RuleName:       rule.Name,
		ProjectID:      rule.ProjectID,
		EventName:      rule.EventName,
		Condition:      rule.Condition,
		CurrentCount:   rule.ThresholdCount + 1,
		ThresholdCount: rule.ThresholdCount,
		WindowMinutes:  rule.WindowMinutes,
		TriggeredAt:    time.Now().UTC().Format(time.RFC3339),
		IsTest:         true,
	}

	return s.dispatcher.DispatchRule(ctx, *rule, payload)
}

func (s *AlertingService) SnoozeRule(ctx context.Context, ruleID uuid.UUID, durationHours int) (*AlertRule, error) {
	var snoozedUntil *time.Time
	if durationHours > 0 {
		until := time.Now().UTC().Add(time.Duration(durationHours) * time.Hour)
		snoozedUntil = &until
	}
	return s.repo.SnoozeRule(ctx, ruleID, snoozedUntil)
}

// TrackEventAndEvaluate records event occurrence into a Redis sliding-window sorted set
// and evaluates against active rules for the project.
func (s *AlertingService) TrackEventAndEvaluate(ctx context.Context, projectID uuid.UUID, eventName string) error {
	if s.redisCli == nil {
		return nil
	}

	now := time.Now().UTC()
	nowUnix := float64(now.Unix())

	// Sorted set key for this project & event: alert:events:<projectID>:<eventName>
	eventSetKey := fmt.Sprintf("alert:events:%s:%s", projectID.String(), eventName)
	member := fmt.Sprintf("%d:%s", now.UnixNano(), uuid.New().String()[:8])

	pipe := s.redisCli.Pipeline()
	pipe.ZAdd(ctx, eventSetKey, redis.Z{Score: nowUnix, Member: member})
	// Keep up to 65 minutes of history for window calculations
	minScore := strconv.FormatFloat(nowUnix-3900, 'f', 0, 64)
	pipe.ZRemRangeByScore(ctx, eventSetKey, "-inf", minScore)
	pipe.Expire(ctx, eventSetKey, 2*time.Hour)
	if _, err := pipe.Exec(ctx); err != nil {
		s.logger.Warn("failed to record event sliding window in redis", slog.String("error", err.Error()))
	}

	rules, err := s.repo.ListActiveRulesForProject(ctx, projectID)
	if err != nil {
		return err
	}

	for _, rule := range rules {
		// Rule matches if event_name is '*' or matches specific event
		if rule.EventName != "*" && rule.EventName != eventName {
			continue
		}

		// Snooze check: suppress evaluation if snoozed_until is still in the future
		if rule.SnoozedUntil != nil && now.Before(*rule.SnoozedUntil) {
			continue
		}

		windowSec := int64(rule.WindowMinutes * 60)
		if windowSec <= 0 {
			windowSec = 300 // default 5m
		}
		windowStart := strconv.FormatFloat(float64(now.Unix()-windowSec), 'f', 0, 64)

		count, err := s.redisCli.ZCount(ctx, eventSetKey, windowStart, "+inf").Result()
		if err != nil {
			s.logger.Warn("failed to count events in redis window", slog.String("error", err.Error()))
			continue
		}

		if EvaluateAlert(rule, count) {
			s.EvaluateAndTrigger(ctx, rule, count, now)
		} else if rule.Status == "firing" {
			// Auto-recovery notification when count drops below breach threshold
			s.triggerRecovery(ctx, rule, count, now)
		}
	}

	return nil
}

func (s *AlertingService) EvaluateAndTrigger(ctx context.Context, rule AlertRule, currentCount int64, now time.Time) {
	ruleUUID, err := uuid.Parse(rule.ID)
	if err != nil {
		return
	}

	// Check cooldown window
	cooldownDuration := time.Duration(rule.CooldownMinutes) * time.Minute
	if cooldownDuration <= 0 {
		cooldownDuration = 15 * time.Minute
	}
	if rule.LastTriggeredAt != nil && now.Sub(*rule.LastTriggeredAt) < cooldownDuration {
		// Cooldown active, suppress alert
		return
	}

	// Prevent distributed concurrent triggers using Redis lock
	lockKey := fmt.Sprintf("lock:alert_rule:%s", rule.ID)
	locked, _ := s.redisCli.SetNX(ctx, lockKey, "1", cooldownDuration).Result()
	if !locked {
		return
	}

	payload := DispatchPayload{
		RuleID:         rule.ID,
		RuleName:       rule.Name,
		ProjectID:      rule.ProjectID,
		EventName:      rule.EventName,
		Condition:      rule.Condition,
		CurrentCount:   currentCount,
		ThresholdCount: rule.ThresholdCount,
		WindowMinutes:  rule.WindowMinutes,
		TriggeredAt:    now.Format(time.RFC3339),
		IsTest:         false,
	}

	go func(r AlertRule, p DispatchPayload, triggerTime time.Time) {
		bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := s.dispatcher.DispatchRule(bgCtx, r, p); err != nil {
			s.logger.Error("failed to dispatch alert notification",
				slog.String("rule_id", r.ID),
				slog.String("channel_type", r.ChannelType),
				slog.String("error", err.Error()),
			)
			return
		}

		_ = s.repo.UpdateLastTriggered(bgCtx, ruleUUID, triggerTime)
	}(rule, payload, now)
}

func (s *AlertingService) triggerRecovery(ctx context.Context, rule AlertRule, currentCount int64, now time.Time) {
	ruleUUID, err := uuid.Parse(rule.ID)
	if err != nil {
		return
	}

	payload := DispatchPayload{
		RuleID:         rule.ID,
		RuleName:       rule.Name,
		ProjectID:      rule.ProjectID,
		EventName:      rule.EventName,
		Condition:      rule.Condition,
		CurrentCount:   currentCount,
		ThresholdCount: rule.ThresholdCount,
		WindowMinutes:  rule.WindowMinutes,
		TriggeredAt:    now.Format(time.RFC3339),
		IsRecovery:     true,
	}

	go func(r AlertRule, p DispatchPayload) {
		bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := s.dispatcher.DispatchRule(bgCtx, r, p); err != nil {
			s.logger.Error("failed to dispatch recovery notification",
				slog.String("rule_id", r.ID),
				slog.String("channel_type", r.ChannelType),
				slog.String("error", err.Error()),
			)
			return
		}

		_ = s.repo.UpdateRuleStatus(bgCtx, ruleUUID, "healthy")
	}(rule, payload)
}
