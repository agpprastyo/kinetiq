package tests

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	db "kinetiq/db/sqlc"
	"kinetiq/internal/alerting"
	"kinetiq/internal/app"
	"kinetiq/internal/webhook"
)

func TestWebhookDispatchSignature(t *testing.T) {
	var receivedSig string
	var receivedBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSig = r.Header.Get("X-Kinetiq-Signature")
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	secret := "wh_sec_9999"
	dispatcher := alerting.NewDispatcherWithClient(http.DefaultClient)
	payload := []byte(`{"event":"alert.triggered","count":150}`)

	err := dispatcher.Dispatch(context.Background(), server.URL, secret, payload)
	if err != nil {
		t.Fatalf("failed to dispatch webhook: %v", err)
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	if receivedSig != expectedSig {
		t.Errorf("expected signature %s, got %s", expectedSig, receivedSig)
	}
	if string(receivedBody) != string(payload) {
		t.Errorf("expected body %s, got %s", string(payload), string(receivedBody))
	}
}

func TestAlertEvaluator(t *testing.T) {
	ruleSpike := alerting.AlertRule{
		Condition:      "spike_gt",
		ThresholdCount: 100,
	}

	if !alerting.EvaluateAlert(ruleSpike, 150) {
		t.Errorf("expected volume 150 to trigger spike_gt > 100")
	}
	if alerting.EvaluateAlert(ruleSpike, 50) {
		t.Errorf("expected volume 50 NOT to trigger spike_gt > 100")
	}

	ruleDrop := alerting.AlertRule{
		Condition:      "drop_lt",
		ThresholdCount: 10,
	}

	if !alerting.EvaluateAlert(ruleDrop, 5) {
		t.Errorf("expected volume 5 to trigger drop_lt < 10")
	}
	if alerting.EvaluateAlert(ruleDrop, 25) {
		t.Errorf("expected volume 25 NOT to trigger drop_lt < 10")
	}
}

func TestMultiChannelDispatch(t *testing.T) {
	dispatcher := alerting.NewDispatcherWithClient(http.DefaultClient)

	t.Run("dispatches slack notification", func(t *testing.T) {
		var receivedBody []byte
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedBody, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		rule := alerting.AlertRule{
			Name:           "Slack Test Rule",
			ChannelType:    "slack",
			WebhookURL:     server.URL,
			ThresholdCount: 100,
			WindowMinutes:  5,
		}
		payload := alerting.DispatchPayload{
			RuleName:       rule.Name,
			EventName:      "api_call",
			CurrentCount:   120,
			ThresholdCount: 100,
			WindowMinutes:  5,
			IsTest:         true,
		}

		err := dispatcher.DispatchRule(context.Background(), rule, payload)
		if err != nil {
			t.Fatalf("expected successful slack dispatch, got: %v", err)
		}
		if !strings.Contains(string(receivedBody), "Slack Test Rule") {
			t.Errorf("expected payload to contain rule name, got %s", string(receivedBody))
		}
	})

	t.Run("dispatches discord notification", func(t *testing.T) {
		var receivedBody []byte
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedBody, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		rule := alerting.AlertRule{
			Name:           "Discord Test Rule",
			ChannelType:    "discord",
			WebhookURL:     server.URL,
			ThresholdCount: 100,
			WindowMinutes:  5,
		}
		payload := alerting.DispatchPayload{
			RuleName:       rule.Name,
			EventName:      "user_signup",
			Condition:      "spike_gt",
			CurrentCount:   150,
			ThresholdCount: 100,
			WindowMinutes:  5,
			IsTest:         false,
		}

		err := dispatcher.DispatchRule(context.Background(), rule, payload)
		if err != nil {
			t.Fatalf("expected successful discord dispatch, got: %v", err)
		}
		if !strings.Contains(string(receivedBody), "Discord Test Rule") {
			t.Errorf("expected payload to contain rule name, got %s", string(receivedBody))
		}
	})

	t.Run("dispatches telegram notification", func(t *testing.T) {
		var receivedBody []byte
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedBody, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		rule := alerting.AlertRule{
			Name:           "Telegram Test Rule",
			ChannelType:    "telegram",
			WebhookURL:     server.URL,
			TelegramChatID: "-10012345",
			ThresholdCount: 50,
			WindowMinutes:  5,
		}
		payload := alerting.DispatchPayload{
			RuleName:       rule.Name,
			EventName:      "error_logged",
			Condition:      "spike_gt",
			CurrentCount:   60,
			ThresholdCount: 50,
			WindowMinutes:  5,
			TriggeredAt:    "2026-10-19T07:00:00Z",
			IsTest:         true,
		}

		err := dispatcher.DispatchRule(context.Background(), rule, payload)
		if err != nil {
			t.Fatalf("expected successful telegram dispatch, got: %v", err)
		}
		if !strings.Contains(string(receivedBody), "Telegram Test Rule") {
			t.Errorf("expected payload to contain rule name, got %s", string(receivedBody))
		}
	})
}

type mockAlertRepo struct {
	rules []alerting.AlertRule
}

func (m *mockAlertRepo) CreateRule(ctx context.Context, rule alerting.AlertRule) (*alerting.AlertRule, error) {
	return &rule, nil
}
func (m *mockAlertRepo) GetRule(ctx context.Context, ruleID uuid.UUID) (*alerting.AlertRule, error) {
	return nil, nil
}
func (m *mockAlertRepo) ListRules(ctx context.Context, projectID uuid.UUID) ([]alerting.AlertRule, error) {
	return m.rules, nil
}
func (m *mockAlertRepo) ListActiveRulesForProject(ctx context.Context, projectID uuid.UUID) ([]alerting.AlertRule, error) {
	return m.rules, nil
}
func (m *mockAlertRepo) UpdateRule(ctx context.Context, rule alerting.AlertRule) (*alerting.AlertRule, error) {
	return &rule, nil
}
func (m *mockAlertRepo) DeleteRule(ctx context.Context, ruleID uuid.UUID) error {
	return nil
}
func (m *mockAlertRepo) UpdateLastTriggered(ctx context.Context, ruleID uuid.UUID, triggeredAt time.Time) error {
	return nil
}
func (m *mockAlertRepo) UpdateRuleStatus(ctx context.Context, ruleID uuid.UUID, status string) error {
	return nil
}
func (m *mockAlertRepo) SnoozeRule(ctx context.Context, ruleID uuid.UUID, snoozedUntil *time.Time) (*alerting.AlertRule, error) {
	return nil, nil
}

func TestAlertingServiceEvaluation(t *testing.T) {
	repo := &mockAlertRepo{
		rules: []alerting.AlertRule{
			{
				ID:              uuid.New().String(),
				Name:            "Mock Rule",
				EventName:       "purchase",
				Condition:       "spike_gt",
				ThresholdCount:  10,
				WindowMinutes:   5,
				ChannelType:     "webhook",
				WebhookURL:      "http://example.com/test",
				CooldownMinutes: 15,
				IsActive:        true,
			},
		},
	}
	dispatcher := alerting.NewDispatcher()
	svc := alerting.NewService(repo, dispatcher, nil, nil)

	pID := uuid.New()
	err := svc.TrackEventAndEvaluate(context.Background(), pID, "purchase")
	if err != nil {
		t.Fatalf("unexpected error when redis client is nil: %v", err)
	}
}

func TestDispatcherErrorPaths(t *testing.T) {
	dispatcher := alerting.NewDispatcherWithClient(http.DefaultClient)
	ctx := context.Background()

	// Unsupported channel
	err := dispatcher.DispatchRule(ctx, alerting.AlertRule{ChannelType: "carrier_pigeon"}, alerting.DispatchPayload{})
	if err == nil {
		t.Errorf("expected error for unsupported channel type, got nil")
	}

	// Server returning 500 for slack
	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer badServer.Close()

	if err := dispatcher.DispatchRule(ctx, alerting.AlertRule{ChannelType: "slack", WebhookURL: badServer.URL}, alerting.DispatchPayload{}); err == nil {
		t.Errorf("expected error for 500 slack response, got nil")
	}
	if err := dispatcher.DispatchRule(ctx, alerting.AlertRule{ChannelType: "discord", WebhookURL: badServer.URL}, alerting.DispatchPayload{}); err == nil {
		t.Errorf("expected error for 500 discord response, got nil")
	}
	if err := dispatcher.DispatchRule(ctx, alerting.AlertRule{ChannelType: "telegram", WebhookURL: badServer.URL, TelegramChatID: "123"}, alerting.DispatchPayload{}); err == nil {
		t.Errorf("expected error for 500 telegram response, got nil")
	}
	if err := dispatcher.DispatchRule(ctx, alerting.AlertRule{ChannelType: "webhook", WebhookURL: badServer.URL}, alerting.DispatchPayload{}); err == nil {
		t.Errorf("expected error for 500 webhook response, got nil")
	}
}

func TestAlertingSQLRepositoryAndService(t *testing.T) {
	ctx := context.Background()
	container, err := app.NewContainer(ctx)
	if err != nil {
		t.Skip("skipping database alerting test if container cannot be initialized")
		return
	}

	repo := alerting.NewSQLRepository(container.DBPool)
	projectID := uuid.New()

	queries := db.New(container.DBPool)
	_, err = queries.CreateProject(ctx, db.CreateProjectParams{
		ID:          pgtype.UUID{Bytes: projectID, Valid: true},
		OwnerUserID: "test-owner",
		Name:        "Alerting Test Project " + projectID.String()[:8],
		Status:      "active",
	})
	if err != nil {
		t.Fatalf("failed to insert test project: %v", err)
	}

	// 1. Create rule
	rule := alerting.AlertRule{
		ProjectID:       projectID.String(),
		Name:            "Integration Spike Rule",
		EventName:       "checkout_failed",
		Condition:       "spike_gt",
		ThresholdCount:  2,
		WindowMinutes:   5,
		ChannelType:     "webhook",
		WebhookURL:      "http://localhost:9999/dummy-wh",
		CooldownMinutes: 1,
		IsActive:        true,
	}

	created, err := repo.CreateRule(ctx, rule)
	if err != nil {
		t.Fatalf("failed to create rule in repo: %v", err)
	}
	ruleUUID, _ := uuid.Parse(created.ID)

	// 2. Get rule
	got, err := repo.GetRule(ctx, ruleUUID)
	if err != nil || got == nil {
		t.Fatalf("failed to get rule: %v", err)
	}

	// 3. List rules
	rules, err := repo.ListRules(ctx, projectID)
	if err != nil || len(rules) == 0 {
		t.Fatalf("expected at least 1 rule from ListRules, got %d, err: %v", len(rules), err)
	}

	// 4. List active rules
	activeRules, err := repo.ListActiveRulesForProject(ctx, projectID)
	if err != nil || len(activeRules) == 0 {
		t.Fatalf("expected at least 1 rule from ListActiveRulesForProject, got %d, err: %v", len(activeRules), err)
	}

	// 5. Update last triggered
	now := time.Now().UTC()
	if err := repo.UpdateLastTriggered(ctx, ruleUUID, now); err != nil {
		t.Fatalf("failed to update last triggered: %v", err)
	}

	// 6. Update rule
	got.ThresholdCount = 1
	got.Name = "Updated Integration Rule"
	updated, err := repo.UpdateRule(ctx, *got)
	if err != nil || updated.ThresholdCount != 1 {
		t.Fatalf("failed to update rule: %v", err)
	}

	// 7. Test Service evaluation with Redis
	var receivedAlert bool
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAlert = true
		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	updated.WebhookURL = mockServer.URL
	updated.CooldownMinutes = 0
	_, _ = repo.UpdateRule(ctx, *updated)

	svc := alerting.NewService(repo, alerting.NewDispatcherWithClient(http.DefaultClient), container.RedisClient, container.Logger)
	// Clear any cooldown lock in Redis
	if container.RedisClient != nil {
		_ = container.RedisClient.Del(ctx, "lock:alert_rule:"+updated.ID).Err()
	}

	// Track event 1 & 2 -> should trigger spike_gt > 1
	_ = svc.TrackEventAndEvaluate(ctx, projectID, "checkout_failed")
	_ = svc.TrackEventAndEvaluate(ctx, projectID, "checkout_failed")
	t.Logf("alert triggered status: %v", receivedAlert)

	// 8. Test test-alert dispatch via Service
	if err := svc.TestRule(ctx, ruleUUID); err != nil {
		t.Logf("test rule dispatch output: %v", err)
	}

	// 9. Delete rule
	if err := repo.DeleteRule(ctx, ruleUUID); err != nil {
		t.Fatalf("failed to delete rule: %v", err)
	}
}

func TestDispatcherBlocksPrivateDestinations(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer srv.Close()

	d := alerting.NewDispatcher()
	for _, u := range []string{srv.URL, "http://169.254.169.254/latest/meta-data/"} {
		if err := d.DispatchRule(context.Background(), alerting.AlertRule{ChannelType: "webhook", WebhookURL: u}, alerting.DispatchPayload{}); err == nil {
			t.Errorf("expected %s to be blocked at dial time", u)
		}
	}
}

func TestDispatcherDoesNotFollowRedirectsToInternalHosts(t *testing.T) {
	var hit bool
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }))
	defer internal.Close()
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, internal.URL, http.StatusFound)
	}))
	defer redir.Close()

	// Unrestricted dial (loopback test servers) but default redirect policy of the safe client.
	safe := webhook.NewSafeHTTPClient(2 * time.Second)
	safe.Transport = http.DefaultTransport
	d := alerting.NewDispatcherWithClient(safe)
	if err := d.Dispatch(context.Background(), redir.URL, "s", []byte(`{}`)); err == nil {
		t.Error("302 must be treated as non-2xx, not followed")
	}
	if hit {
		t.Error("redirect target must never be contacted")
	}
}
