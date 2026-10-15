// Package audit records security and financial mutations on the audit_logs table.
package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Entry is one audit record. ActionCode and TargetType are required.
type Entry struct {
	ActorUserID *uuid.UUID
	ActionCode  string
	TargetType  string
	TargetID    *uuid.UUID
	ReasonCode  *string
	RequestID   *uuid.UUID
	Metadata    map[string]interface{}
}

// Service writes structured audit entries.
type Service struct {
	pool *pgxpool.Pool
}

// NewService constructs the audit service. A nil pool yields a service whose writes
// return an error, so a caller can decide between failing the mutation or logging.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// Log persists one audit entry. Empty action_code or target_type is rejected so the
// table never accumulates rows that cannot be traced back to a specific operation.
func (s *Service) Log(ctx context.Context, entry Entry) error {
	if entry.ActionCode == "" {
		return errors.New("audit action_code is required")
	}
	if entry.TargetType == "" {
		return errors.New("audit target_type is required")
	}
	if s.pool == nil {
		return errors.New("audit service has no database pool")
	}

	metadata := entry.Metadata
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to encode audit metadata: %w", err)
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO audit_logs (actor_user_id, action_code, target_type, target_id, reason_code, request_id, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)
	`, entry.ActorUserID, entry.ActionCode, entry.TargetType, entry.TargetID, entry.ReasonCode, entry.RequestID, string(raw))
	if err != nil {
		return fmt.Errorf("failed to write audit entry: %w", err)
	}
	return nil
}

// Logger is the minimal surface the usecases depend on, so tests can substitute a fake.
type Logger interface {
	Log(ctx context.Context, entry Entry) error
}
