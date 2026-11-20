package compliance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AuditService struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

func NewAuditService(pool *pgxpool.Pool, logger *slog.Logger) *AuditService {
	return &AuditService{
		pool:   pool,
		logger: logger,
	}
}

func (s *AuditService) Record(ctx context.Context, params RecordAuditParams) error {
	if params.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if params.ActorID == "" || params.Action == "" || params.TargetType == "" || params.TargetID == "" {
		return fmt.Errorf("actor_id, action, target_type, and target_id are required")
	}

	metaBytes, err := json.Marshal(params.Metadata)
	if err != nil {
		metaBytes = []byte("{}")
	}

	query := `
		INSERT INTO workspace_audit_logs (
			workspace_id, project_id, actor_id, actor_name, actor_email,
			action, target_type, target_id, metadata, ip_address, user_agent
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
		);
	`

	_, err = s.pool.Exec(ctx, query,
		params.WorkspaceID,
		params.ProjectID,
		params.ActorID,
		params.ActorName,
		params.ActorEmail,
		params.Action,
		params.TargetType,
		params.TargetID,
		metaBytes,
		params.IPAddress,
		params.UserAgent,
	)
	if err != nil {
		if s.logger != nil {
			s.logger.ErrorContext(ctx, "failed to record workspace audit log", slog.String("error", err.Error()))
		}
		return fmt.Errorf("failed to insert audit log: %w", err)
	}

	return nil
}

func (s *AuditService) List(ctx context.Context, params ListAuditLogsParams) (*ListAuditLogsResponse, error) {
	if params.WorkspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}

	if params.Limit <= 0 {
		params.Limit = 50
	}
	if params.Limit > 100 {
		params.Limit = 100
	}
	if params.Offset < 0 {
		params.Offset = 0
	}

	var conditions []string
	var args []interface{}
	argIdx := 1

	conditions = append(conditions, fmt.Sprintf("workspace_id = $%d", argIdx))
	args = append(args, params.WorkspaceID)
	argIdx++

	if params.ProjectID != nil && *params.ProjectID != uuid.Nil {
		conditions = append(conditions, fmt.Sprintf("project_id = $%d", argIdx))
		args = append(args, *params.ProjectID)
		argIdx++
	}

	if params.Action != "" {
		conditions = append(conditions, fmt.Sprintf("action = $%d", argIdx))
		args = append(args, params.Action)
		argIdx++
	}

	if params.TargetType != "" {
		conditions = append(conditions, fmt.Sprintf("target_type = $%d", argIdx))
		args = append(args, params.TargetType)
		argIdx++
	}

	if params.Search != "" {
		conditions = append(conditions, fmt.Sprintf("(actor_email ILIKE $%d OR actor_name ILIKE $%d OR target_id ILIKE $%d)", argIdx, argIdx, argIdx))
		args = append(args, "%"+params.Search+"%")
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")

	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM workspace_audit_logs WHERE %s;", whereClause)
	var total int64
	if err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("failed to count audit logs: %w", err)
	}

	// Select items
	selectQuery := fmt.Sprintf(`
		SELECT id, workspace_id, project_id, actor_id, actor_name, actor_email,
		       action, target_type, target_id, metadata, ip_address, user_agent, created_at
		FROM workspace_audit_logs
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d;
	`, whereClause, argIdx, argIdx+1)

	args = append(args, params.Limit, params.Offset)

	rows, err := s.pool.Query(ctx, selectQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit logs: %w", err)
	}
	defer rows.Close()

	var items []WorkspaceAuditLog
	for rows.Next() {
		var item WorkspaceAuditLog
		var metaBytes []byte
		var pID *uuid.UUID

		err := rows.Scan(
			&item.ID,
			&item.WorkspaceID,
			&pID,
			&item.ActorID,
			&item.ActorName,
			&item.ActorEmail,
			&item.Action,
			&item.TargetType,
			&item.TargetID,
			&metaBytes,
			&item.IPAddress,
			&item.UserAgent,
			&item.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan audit log row: %w", err)
		}
		item.ProjectID = pID
		if len(metaBytes) > 0 {
			_ = json.Unmarshal(metaBytes, &item.Metadata)
		}
		if item.Metadata == nil {
			item.Metadata = make(map[string]interface{})
		}
		items = append(items, item)
	}

	page := (params.Offset / params.Limit) + 1

	return &ListAuditLogsResponse{
		Items: items,
		Total: total,
		Page:  page,
		Limit: params.Limit,
	}, nil
}

func (s *AuditService) GDPRDelete(ctx context.Context, params GDPRDeleteParams) (*GDPRDeleteResult, error) {
	if params.ProjectID == uuid.Nil {
		return nil, fmt.Errorf("project_id is required")
	}
	if strings.TrimSpace(params.DistinctID) == "" {
		return nil, fmt.Errorf("distinct_id is required")
	}

	mode := strings.ToLower(strings.TrimSpace(params.Mode))
	if mode == "" {
		mode = "delete"
	}

	var affected int64
	var pseudonymID string

	switch mode {
	case "pseudonymize":
		h := sha256.Sum256([]byte(params.DistinctID + params.ProjectID.String()))
		pseudonymID = "anon_" + hex.EncodeToString(h[:8])
		query := `
			UPDATE events
			SET distinct_id = $1,
			    properties = properties - 'email' - 'name' - 'phone' - 'ip'
			WHERE project_id = $2 AND distinct_id = $3;
		`
		tag, err := s.pool.Exec(ctx, query, pseudonymID, params.ProjectID, params.DistinctID)
		if err != nil {
			if s.logger != nil {
				s.logger.ErrorContext(ctx, "failed to pseudonymize user events", slog.String("error", err.Error()))
			}
			return nil, fmt.Errorf("failed to pseudonymize events: %w", err)
		}
		affected = tag.RowsAffected()

	case "delete":
		query := `DELETE FROM events WHERE project_id = $1 AND distinct_id = $2;`
		tag, err := s.pool.Exec(ctx, query, params.ProjectID, params.DistinctID)
		if err != nil {
			if s.logger != nil {
				s.logger.ErrorContext(ctx, "failed to delete user events", slog.String("error", err.Error()))
			}
			return nil, fmt.Errorf("failed to delete events: %w", err)
		}
		affected = tag.RowsAffected()

	default:
		return nil, fmt.Errorf("unsupported mode: %s (must be 'delete' or 'pseudonymize')", mode)
	}

	// Always record audit trail
	workspaceID := params.WorkspaceID
	if workspaceID == "" {
		workspaceID = params.ActorID
	}
	meta := map[string]interface{}{
		"mode":          mode,
		"affected_rows": affected,
	}
	if pseudonymID != "" {
		meta["pseudonym_id"] = pseudonymID
	}

	_ = s.Record(ctx, RecordAuditParams{
		WorkspaceID: workspaceID,
		ProjectID:   &params.ProjectID,
		ActorID:     params.ActorID,
		ActorName:   params.ActorName,
		ActorEmail:  params.ActorEmail,
		Action:      "compliance.gdpr_delete",
		TargetType:  "user_data",
		TargetID:    params.DistinctID,
		Metadata:    meta,
		IPAddress:   params.IPAddress,
		UserAgent:   params.UserAgent,
	})

	return &GDPRDeleteResult{
		ProjectID:    params.ProjectID,
		DistinctID:   params.DistinctID,
		Mode:         mode,
		AffectedRows: affected,
		PseudonymID:  pseudonymID,
	}, nil
}
