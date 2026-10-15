package audit_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/galeriku/core/internal/audit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func liveAuditPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping live audit test in short mode")
	}
	host := os.Getenv("DB_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("DB_PORT")
	if port == "" {
		port = "5433"
	}
	user := os.Getenv("DB_USER")
	if user == "" {
		user = "galeriku"
	}
	pass := os.Getenv("DB_PASSWORD")
	if pass == "" {
		pass = "galeriku_secret_pass"
	}
	name := os.Getenv("DB_NAME")
	if name == "" {
		name = "galeriku_db"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, pass, host, port, name))
	if err != nil {
		t.Skipf("cannot connect to postgres: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("postgres ping failed: %v", err)
	}
	return pool
}

func TestAuditService_WritesStructuredEntry(t *testing.T) {
	pool := liveAuditPool(t)
	defer pool.Close()

	svc := audit.NewService(pool)
	ctx := context.Background()

	actor := uuid.Must(uuid.NewV7())
	target := uuid.Must(uuid.NewV7())

	err := svc.Log(ctx, audit.Entry{
		ActorUserID: &actor,
		ActionCode:  "TEST_AUDIT_ENTRY",
		TargetType:  "test_target",
		TargetID:    &target,
		Metadata:    map[string]interface{}{"note": "unit test"},
	})
	if err != nil {
		t.Fatalf("failed to write audit entry: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM audit_logs WHERE action_code = 'TEST_AUDIT_ENTRY'")
	}()

	var gotActor uuid.UUID
	var actionCode, targetType string
	var gotTarget uuid.UUID
	var metadata []byte
	err = pool.QueryRow(ctx, `
		SELECT actor_user_id, action_code, target_type, target_id, metadata
		FROM audit_logs WHERE action_code = 'TEST_AUDIT_ENTRY'`).Scan(&gotActor, &actionCode, &targetType, &gotTarget, &metadata)
	if err != nil {
		t.Fatalf("failed to read audit entry: %v", err)
	}
	if gotActor != actor || gotTarget != target || targetType != "test_target" {
		t.Fatalf("audit entry mismatch: actor=%s target=%s type=%s", gotActor, gotTarget, targetType)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(metadata, &parsed); err != nil {
		t.Fatalf("metadata is not valid JSON: %v", err)
	}
	if parsed["note"] != "unit test" {
		t.Fatalf("expected metadata note, got %v", parsed)
	}
}

func TestAuditService_RequiresActionCode(t *testing.T) {
	pool := liveAuditPool(t)
	defer pool.Close()

	svc := audit.NewService(pool)
	if err := svc.Log(context.Background(), audit.Entry{TargetType: "test_target"}); err == nil {
		t.Fatal("expected rejection when action_code is empty")
	}
	if err := svc.Log(context.Background(), audit.Entry{ActionCode: "TEST"}); err == nil {
		t.Fatal("expected rejection when target_type is empty")
	}
}
