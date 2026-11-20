package export

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockStorageDriver struct {
	mock.Mock
}

func (m *mockStorageDriver) TestConnection(ctx context.Context, cfg StorageConfig) error {
	args := m.Called(ctx, cfg)
	return args.Error(0)
}

func (m *mockStorageDriver) UploadGzip(ctx context.Context, cfg StorageConfig, key string, data io.Reader) error {
	args := m.Called(ctx, cfg, key, data)
	return args.Error(0)
}

func TestSyncWorker_PartitionKey(t *testing.T) {
	worker := NewSyncWorker(nil, nil, nil)
	projectID := uuid.MustParse("a0000000-0000-0000-0000-000000000001")
	syncTime := time.Date(2026, 10, 23, 14, 30, 0, 0, time.UTC)

	key := worker.BuildPartitionKey(projectID, syncTime)
	expected := "kinetiq/project_id=a0000000-0000-0000-0000-000000000001/year=2026/month=10/day=23/events-1400.jsonl.gz"
	assert.Equal(t, expected, key)
}

func TestSyncWorker_CompressAndUpload(t *testing.T) {
	ctx := context.Background()
	mockDriver := new(mockStorageDriver)

	var capturedData []byte
	mockDriver.On("UploadGzip", ctx, mock.Anything, "test.jsonl.gz", mock.Anything).
		Run(func(args mock.Arguments) {
			r := args.Get(3).(io.Reader)
			data, _ := io.ReadAll(r)
			capturedData = data
		}).
		Return(nil)

	worker := NewSyncWorker(nil, mockDriver, nil)
	lines := [][]byte{
		[]byte(`{"event":"page_view","id":1}`),
		[]byte(`{"event":"click","id":2}`),
	}

	cfg := StorageConfig{BucketName: "test-bucket"}
	err := worker.CompressAndUpload(ctx, cfg, "test.jsonl.gz", lines)
	assert.NoError(t, err)

	// Validate gzip content decompression
	gzReader, err := gzip.NewReader(bytes.NewReader(capturedData))
	assert.NoError(t, err)
	decompressed, err := io.ReadAll(gzReader)
	assert.NoError(t, err)

	expectedStr := "{\"event\":\"page_view\",\"id\":1}\n{\"event\":\"click\",\"id\":2}\n"
	assert.Equal(t, expectedStr, string(decompressed))
	mockDriver.AssertExpectations(t)
}

func TestSyncWorker_MaskSecret(t *testing.T) {
	secret := "my-super-secret-aws-key"
	masked := MaskSecret(secret)
	assert.Equal(t, "********", masked)

	projectSync := ProjectStorageSync{
		ID:              uuid.New(),
		Provider:        "s3",
		BucketName:      "vault",
		SecretAccessKey: "supersecret",
	}

	resp := SerializeSyncResponse(projectSync)
	assert.Equal(t, "********", resp.SecretAccessKey)
}
