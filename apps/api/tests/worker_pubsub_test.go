package tests

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"kinetiq/internal/app"
	"kinetiq/internal/worker"
)

func TestWorkerPublishesToProjectChannel(t *testing.T) {
	os.Setenv("AUTH_INTERNAL_SECRET", "test-internal-secret")
	os.Setenv("AUTH_BASE_URL", "http://localhost:3001")

	ctx := context.Background()
	container, err := app.NewContainer(ctx)
	if err != nil {
		t.Fatalf("failed to create container: %v", err)
	}

	rdb := container.RedisClient
	projectID := uuid.New()
	channel := "events:pubsub:" + projectID.String()

	pubsub := rdb.Subscribe(ctx, channel)
	defer pubsub.Close()

	// Wait for subscription confirmation
	_, err = pubsub.Receive(ctx)
	if err != nil {
		t.Fatalf("failed to subscribe to channel: %v", err)
	}

	testEvent := worker.PublishedEvent{
		EventID:   uuid.New().String(),
		ProjectID: projectID.String(),
		Name:      "user_registered",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	err = worker.PublishLiveEvent(ctx, rdb, projectID.String(), testEvent)
	if err != nil {
		t.Fatalf("failed to publish live event: %v", err)
	}

	msg, err := pubsub.ReceiveMessage(ctx)
	if err != nil {
		t.Fatalf("failed to receive pubsub message: %v", err)
	}

	var received worker.PublishedEvent
	err = json.Unmarshal([]byte(msg.Payload), &received)
	if err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}
	if received.Name != "user_registered" {
		t.Errorf("expected event name user_registered, got %s", received.Name)
	}
	if received.ProjectID != projectID.String() {
		t.Errorf("expected project ID %s, got %s", projectID.String(), received.ProjectID)
	}
}
