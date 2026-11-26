package person

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidID         = errors.New("invalid or reserved distinct id")
	ErrAlreadyIdentified = errors.New("cannot merge two already-identified persons")
)

const maxIDLen = 128

var reserved = map[string]struct{}{
	"": {}, "null": {}, "undefined": {}, "nan": {}, "none": {}, "nil": {}, "0": {}, "-1": {},
	"anonymous": {}, "guest": {}, "[object object]": {}, "true": {}, "false": {},
}

// ValidateID rejects empty, over-long, and well-known placeholder IDs.
func ValidateID(id string) error {
	if len(id) > maxIDLen {
		return ErrInvalidID
	}
	if _, bad := reserved[strings.ToLower(strings.TrimSpace(id))]; bad {
		return ErrInvalidID
	}
	return nil
}

type Person struct {
	ID           uuid.UUID
	IsIdentified bool
	Properties   map[string]any
}

type IdentifyInput struct {
	DistinctID  string
	AnonymousID string
	Set         map[string]any
	SetOnce     map[string]any
}

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// Identify links AnonymousID (optional) to DistinctID's person and marks it identified.
func (s *Service) Identify(ctx context.Context, projectID uuid.UUID, in IdentifyInput) (*Person, error) {
	if err := ValidateID(in.DistinctID); err != nil {
		return nil, err
	}
	if in.AnonymousID != "" {
		if err := ValidateID(in.AnonymousID); err != nil {
			return nil, err
		}
	}
	var out *Person
	err := s.locked(ctx, projectID, func(tx pgx.Tx) error {
		p, err := ensure(ctx, tx, projectID, in.DistinctID)
		if err != nil {
			return err
		}
		if in.AnonymousID != "" && in.AnonymousID != in.DistinctID {
			anon, err := ensure(ctx, tx, projectID, in.AnonymousID)
			if err != nil {
				return err
			}
			if anon.ID != p.ID && anon.IsIdentified && !p.IsIdentified {
				return ErrAlreadyIdentified
			}
			if p, err = merge(ctx, tx, p, anon); err != nil {
				return err
			}
		}
		out, err = update(ctx, tx, p.ID, in.Set, in.SetOnce, true)
		return err
	})
	return out, err
}

// Alias links two known IDs into one person without marking it identified.
func (s *Service) Alias(ctx context.Context, projectID uuid.UUID, distinctID, alias string) (*Person, error) {
	if err := ValidateID(distinctID); err != nil {
		return nil, err
	}
	if err := ValidateID(alias); err != nil {
		return nil, err
	}
	var out *Person
	err := s.locked(ctx, projectID, func(tx pgx.Tx) error {
		a, err := ensure(ctx, tx, projectID, distinctID)
		if err != nil {
			return err
		}
		b, err := ensure(ctx, tx, projectID, alias)
		if err != nil {
			return err
		}
		out, err = merge(ctx, tx, a, b)
		return err
	})
	return out, err
}

// Capture ensures a person exists for distinctID and applies event-level $set/$set_once.
func (s *Service) Capture(ctx context.Context, projectID uuid.UUID, distinctID string, set, setOnce map[string]any) error {
	if err := ValidateID(distinctID); err != nil {
		return err
	}
	if len(set) == 0 && len(setOnce) == 0 {
		var one int
		err := s.pool.QueryRow(ctx, `SELECT 1 FROM person_distinct_ids WHERE project_id=$1 AND distinct_id=$2`, projectID, distinctID).Scan(&one)
		if err == nil {
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	}
	return s.locked(ctx, projectID, func(tx pgx.Tx) error {
		p, err := ensure(ctx, tx, projectID, distinctID)
		if err != nil {
			return err
		}
		_, err = update(ctx, tx, p.ID, set, setOnce, false)
		return err
	})
}

// locked serializes identity writes per project so merges and ingestion never race.
// ponytail: one advisory lock per project caps write throughput of identity changes; shard by person if hot.
func (s *Service) locked(ctx context.Context, projectID uuid.UUID, fn func(pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "person:"+projectID.String()); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func ensure(ctx context.Context, tx pgx.Tx, projectID uuid.UUID, distinctID string) (*Person, error) {
	var id uuid.UUID
	err := tx.QueryRow(ctx, `SELECT person_id FROM person_distinct_ids WHERE project_id=$1 AND distinct_id=$2`, projectID, distinctID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		if err = tx.QueryRow(ctx, `INSERT INTO persons (project_id) VALUES ($1) RETURNING id`, projectID).Scan(&id); err != nil {
			return nil, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO person_distinct_ids (project_id, distinct_id, person_id) VALUES ($1,$2,$3)`, projectID, distinctID, id); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	return load(ctx, tx, id)
}

func load(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*Person, error) {
	p := &Person{ID: id}
	var props []byte
	if err := tx.QueryRow(ctx, `SELECT is_identified, properties FROM persons WHERE id=$1 FOR UPDATE`, id).Scan(&p.IsIdentified, &props); err != nil {
		return nil, err
	}
	return p, json.Unmarshal(props, &p.Properties)
}

// merge folds the loser into the survivor (identified side wins, else a). Events are not rewritten.
func merge(ctx context.Context, tx pgx.Tx, a, b *Person) (*Person, error) {
	if a.ID == b.ID {
		return a, nil
	}
	if a.IsIdentified && b.IsIdentified {
		return nil, ErrAlreadyIdentified
	}
	win, lose := a, b
	if b.IsIdentified {
		win, lose = b, a
	}
	if _, err := tx.Exec(ctx, `UPDATE person_distinct_ids SET person_id=$1 WHERE person_id=$2`, win.ID, lose.ID); err != nil {
		return nil, err
	}
	loseProps, _ := json.Marshal(lose.Properties)
	if _, err := tx.Exec(ctx, `UPDATE persons SET properties=$2::jsonb || properties, updated_at=NOW() WHERE id=$1`, win.ID, loseProps); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM persons WHERE id=$1`, lose.ID); err != nil {
		return nil, err
	}
	return load(ctx, tx, win.ID)
}

// update applies $set_once (existing wins) then $set (new wins).
func update(ctx context.Context, tx pgx.Tx, id uuid.UUID, set, setOnce map[string]any, identify bool) (*Person, error) {
	setJSON, _ := json.Marshal(orEmpty(set))
	onceJSON, _ := json.Marshal(orEmpty(setOnce))
	if _, err := tx.Exec(ctx, `
		UPDATE persons
		SET properties = ($2::jsonb || properties) || $3::jsonb,
		    is_identified = is_identified OR $4,
		    updated_at = NOW()
		WHERE id = $1`, id, onceJSON, setJSON, identify); err != nil {
		return nil, err
	}
	return load(ctx, tx, id)
}

func orEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}
