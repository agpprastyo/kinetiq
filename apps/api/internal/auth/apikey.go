package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	db "kinetiq/db/sqlc"
)

type APIKeyValidator interface {
	ValidateKey(ctx context.Context, rawKey string) (projectID uuid.UUID, err error)
}

type ProjectKeyValidator struct {
	queries *db.Queries
	redis   *redis.Client
	logger  *slog.Logger
}

func NewProjectKeyValidator(queries *db.Queries, redisClient *redis.Client, logger *slog.Logger) *ProjectKeyValidator {
	return &ProjectKeyValidator{
		queries: queries,
		redis:   redisClient,
		logger:  logger,
	}
}

// ValidateKey validates rawKey (format: tlk_live_<prefix>_<secret> or legacy sp_live_<prefix>_<secret>)
func (v *ProjectKeyValidator) ValidateKey(ctx context.Context, rawKey string) (uuid.UUID, error) {
	if !strings.HasPrefix(rawKey, "tlk_live_") && !strings.HasPrefix(rawKey, "tlk_test_") && !strings.HasPrefix(rawKey, "sp_live_") {
		return uuid.Nil, fmt.Errorf("invalid api key prefix format")
	}

	parts := strings.Split(rawKey, "_")
	if len(parts) != 4 {
		return uuid.Nil, fmt.Errorf("malformed api key format")
	}
	prefix := parts[2]

	// 1. Check Redis Cache first (prefix -> cached JSON with project_id and key_hash)
	cacheKey := fmt.Sprintf("apikey:%s", prefix)
	if v.redis != nil {
		cachedData, err := v.redis.HGetAll(ctx, cacheKey).Result()
		if err == nil && len(cachedData) > 0 {
			cachedHash := cachedData["hash"]
			cachedProjectIDStr := cachedData["project_id"]

			computedHash := sha256Hex(rawKey)
			if subtle.ConstantTimeCompare([]byte(cachedHash), []byte(computedHash)) == 1 {
				pID, parseErr := uuid.Parse(cachedProjectIDStr)
				if parseErr == nil {
					return pID, nil
				}
			}
		}
	}

	// 2. Query Postgres via sqlc
	keyRow, err := v.queries.GetActiveKeyByPrefix(ctx, prefix)
	if err != nil {
		if v.logger != nil {
			v.logger.WarnContext(ctx, "api key prefix lookup failed or revoked", slog.String("prefix", prefix))
		}
		return uuid.Nil, fmt.Errorf("api key not found or revoked")
	}

	// 3. Constant-time hash verification
	computedHash := sha256Hex(rawKey)
	if subtle.ConstantTimeCompare([]byte(keyRow.KeyHash), []byte(computedHash)) != 1 {
		if v.logger != nil {
			v.logger.WarnContext(ctx, "api key hash mismatch", slog.String("prefix", prefix))
		}
		return uuid.Nil, fmt.Errorf("invalid api key secret")
	}

	projectUUID := uuid.UUID(keyRow.ProjectID.Bytes)

	// 4. Cache-aside in Redis (10 minutes TTL)
	if v.redis != nil {
		_ = v.redis.HSet(ctx, cacheKey, map[string]interface{}{
			"hash":       keyRow.KeyHash,
			"project_id": projectUUID.String(),
		}).Err()
		_ = v.redis.Expire(ctx, cacheKey, 10*time.Minute).Err()
	}

	return projectUUID, nil
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
