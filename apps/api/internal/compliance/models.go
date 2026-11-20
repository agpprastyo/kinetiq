package compliance

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type WorkspaceAuditLog struct {
	ID          uuid.UUID              `json:"id"`
	WorkspaceID string                 `json:"workspace_id"`
	ProjectID   *uuid.UUID             `json:"project_id,omitempty"`
	ActorID     string                 `json:"actor_id"`
	ActorName   string                 `json:"actor_name"`
	ActorEmail  string                 `json:"actor_email"`
	Action      string                 `json:"action"`
	TargetType  string                 `json:"target_type"`
	TargetID    string                 `json:"target_id"`
	Metadata    map[string]interface{} `json:"metadata"`
	IPAddress   string                 `json:"ip_address,omitempty"`
	UserAgent   string                 `json:"user_agent,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
}

type RecordAuditParams struct {
	WorkspaceID string                 `json:"workspace_id"`
	ProjectID   *uuid.UUID             `json:"project_id,omitempty"`
	ActorID     string                 `json:"actor_id"`
	ActorName   string                 `json:"actor_name"`
	ActorEmail  string                 `json:"actor_email"`
	Action      string                 `json:"action"`
	TargetType  string                 `json:"target_type"`
	TargetID    string                 `json:"target_id"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	IPAddress   string                 `json:"ip_address,omitempty"`
	UserAgent   string                 `json:"user_agent,omitempty"`
}

type ListAuditLogsParams struct {
	WorkspaceID string     `json:"workspace_id"`
	ProjectID   *uuid.UUID `json:"project_id,omitempty"`
	Action      string     `json:"action,omitempty"`
	TargetType  string     `json:"target_type,omitempty"`
	Search      string     `json:"search,omitempty"`
	Limit       int        `json:"limit"`
	Offset      int        `json:"offset"`
}

type ListAuditLogsResponse struct {
	Items []WorkspaceAuditLog `json:"items"`
	Total int64               `json:"total"`
	Page  int                 `json:"page"`
	Limit int                 `json:"limit"`
}

type GDPRDeleteParams struct {
	WorkspaceID string    `json:"workspace_id"`
	ProjectID   uuid.UUID `json:"project_id"`
	DistinctID  string    `json:"distinct_id"`
	Mode        string    `json:"mode"` // "delete" (hard delete) or "pseudonymize" (anonymize)
	ActorID     string    `json:"actor_id"`
	ActorName   string    `json:"actor_name"`
	ActorEmail  string    `json:"actor_email"`
	IPAddress   string    `json:"ip_address,omitempty"`
	UserAgent   string    `json:"user_agent,omitempty"`
}

type GDPRDeleteResult struct {
	ProjectID    uuid.UUID `json:"project_id"`
	DistinctID   string    `json:"distinct_id"`
	Mode         string    `json:"mode"`
	AffectedRows int64     `json:"affected_rows"`
	PseudonymID  string    `json:"pseudonym_id,omitempty"`
}

type Service interface {
	Record(ctx context.Context, params RecordAuditParams) error
	List(ctx context.Context, params ListAuditLogsParams) (*ListAuditLogsResponse, error)
	GDPRDelete(ctx context.Context, params GDPRDeleteParams) (*GDPRDeleteResult, error)
}
