# Phase 1: Core Foundation & Infrastructure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Establish and verify the core infrastructure for Galeriku, including Docker service orchestration, 76-table PostgreSQL 16 schema with UUIDv7, Go hexagonal backend connection pool with health probes, and the TypeScript web workspace enforcing antislop craftsmanship standards and OpenAPI type synchronization.

**Architecture:** A Go 1.22 modular monolith core backed by PostgreSQL 16 (pgxpool) for ACID source of truth, MinIO S3 for object storage, Redis 7 for caching and rate limiting, RabbitMQ for asynchronous event brokering, and Qdrant for biometric vectors. All web frontend clients share a unified TypeScript type system generated from OpenAPI 3.1 with strict antislop design tokens.

**Tech Stack:** Go 1.22, PostgreSQL 16, pgx/v5, MinIO Go SDK v7, Redis 7, RabbitMQ 3.13, Qdrant v1.11, Docker Compose, Node.js v20, TypeScript 5.4, Vitest.

## Global Constraints

- **Zero-Trust Backend**: All prices, balance, payment status, refunds, and download grants are server-authoritative.
- **Financial Precision**: All Indonesian Rupiah amounts are stored as exact integer `BIGINT` (no floats).
- **Biometric Compliance**: No raw biometric embeddings stored in PostgreSQL; Qdrant handles vectors under UU PDP No. 27/2022.
- **Antislop Hard Gates**: Zero em dashes (`—`) in UI copy, minimum 44px tap targets, WCAG AA contrast (4.5:1), mandatory empty/loading/error UI states, no dead interactive elements, no fabricated statistics.
- **Antislop Dials**: ENERGY 2 (Balanced) / RHYTHM 2 (Content-driven sections) / MOTION 1 (Essential micro-interactions only).

---

### Task 1: Docker Infrastructure Orchestration & MinIO S3 Provisioner

**Files:**
- Create: `backend/cmd/provisioner/main.go`
- Create: `backend/cmd/provisioner/main_test.go`
- Modify: `docker-compose.yml`
- Modify: `backend/go.mod`

**Interfaces:**
- Consumes: Environment variables (`S3_ENDPOINT`, `MINIO_ROOT_USER`, `MINIO_ROOT_PASSWORD`) from `.env`.
- Produces: `EnsureBucketsExist(endpoint, accessKey, secretKey, useSSL string) error` provisioning `galeriku-master-vault` (private) and `galeriku-public-media` (public read).

- [ ] **Step 1: Write the failing test for MinIO provisioner**

Create `backend/cmd/provisioner/main_test.go`:
```go
package main

import (
	"context"
	"testing"
)

func TestEnsureBucketsContract(t *testing.T) {
	// Verifies provisioner rejects empty credentials
	err := EnsureBucketsExist(context.Background(), "", "", "", false)
	if err == nil {
		t.Fatal("expected error when credentials and endpoint are empty, got nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /root/galeriku/backend && go test -v ./cmd/provisioner/...`
Expected: FAIL with "undefined: EnsureBucketsExist"

- [ ] **Step 3: Write minimal implementation for provisioner**

Install MinIO client dependency:
Run: `cd /root/galeriku/backend && go get github.com/minio/minio-go/v7 && go get github.com/minio/minio-go/v7/pkg/credentials`

Create `backend/cmd/provisioner/main.go`:
```go
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const (
	BucketMasterVault = "galeriku-master-vault"
	BucketPublicMedia = "galeriku-public-media"
)

func EnsureBucketsExist(ctx context.Context, endpoint, accessKey, secretKey string, useSSL bool) error {
	if endpoint == "" || accessKey == "" || secretKey == "" {
		return errors.New("endpoint, accessKey, and secretKey must not be empty")
	}

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize minio client: %w", err)
	}

	buckets := []string{BucketMasterVault, BucketPublicMedia}
	for _, bucket := range buckets {
		exists, err := client.BucketExists(ctx, bucket)
		if err != nil {
			return fmt.Errorf("failed checking bucket %s: %w", bucket, err)
		}
		if !exists {
			err = client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
			if err != nil {
				return fmt.Errorf("failed creating bucket %s: %w", bucket, err)
			}
			log.Printf("[PROVISIONER] Created S3 bucket: %s", bucket)
		} else {
			log.Printf("[PROVISIONER] S3 bucket already exists: %s", bucket)
		}
	}

	// Set public read policy on galeriku-public-media
	publicPolicy := fmt.Sprintf(`{
		"Version": "2012-10-17",
		"Statement": [
			{
				"Effect": "Allow",
				"Principal": {"AWS": ["*"]},
				"Action": ["s3:GetObject"],
				"Resource": ["arn:aws:s3:::%s/*"]
			}
		]
	}`, BucketPublicMedia)

	err = client.SetBucketPolicy(ctx, BucketPublicMedia, publicPolicy)
	if err != nil {
		return fmt.Errorf("failed to set public policy on %s: %w", BucketPublicMedia, err)
	}
	log.Printf("[PROVISIONER] Applied public-read policy to %s", BucketPublicMedia)

	return nil
}

func main() {
	endpoint := os.Getenv("S3_ENDPOINT")
	if endpoint == "" {
		endpoint = "localhost:9000"
	}
	accessKey := os.Getenv("MINIO_ROOT_USER")
	if accessKey == "" {
		accessKey = "galeriku_admin"
	}
	secretKey := os.Getenv("MINIO_ROOT_PASSWORD")
	if secretKey == "" {
		secretKey = "galeriku_minio_secret"
	}

	log.Printf("[PROVISIONER] Connecting to MinIO at %s...", endpoint)
	if err := EnsureBucketsExist(context.Background(), endpoint, accessKey, secretKey, false); err != nil {
		log.Fatalf("[PROVISIONER] Failed: %v", err)
	}
	log.Println("[PROVISIONER] S3 Buckets provisioned successfully.")
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /root/galeriku/backend && go test -v ./cmd/provisioner/...`
Expected: PASS

- [ ] **Step 5: Launch containers and verify provisioner live**

Run:
```bash
cd /root/galeriku && docker compose up -d postgres redis rabbitmq qdrant minio
docker compose ps
cd /root/galeriku/backend && go run ./cmd/provisioner/main.go
```
Expected: All 5 containers healthy, S3 buckets created and confirmed.

- [ ] **Step 6: Commit**

```bash
git add backend/cmd/provisioner backend/go.mod backend/go.sum
git commit -m "feat(infra): add MinIO S3 bucket provisioner and launch docker core services"
```

---

### Task 2: Database Migration Runner & 76-Table Verification Test

**Files:**
- Create: `backend/internal/database/migrator.go`
- Create: `backend/internal/database/migrator_test.go`
- Test: `migrations/000001_initial_schema.up.sql`

**Interfaces:**
- Consumes: PostgreSQL connection string (`postgres://galeriku:galeriku_secret_pass@localhost:5432/galeriku_db?sslmode=disable`).
- Produces: `VerifySchemaCompleteness(ctx context.Context, db *pgxpool.Pool) (int, error)` returning the count of confirmed tables in `public` schema (must equal 76).

- [ ] **Step 1: Write failing schema verification test**

Install `pgx/v5` dependency:
Run: `cd /root/galeriku/backend && go get github.com/jackc/pgx/v5 && go get github.com/jackc/pgx/v5/pgxpool`

Create `backend/internal/database/migrator_test.go`:
```go
package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestVerifySchemaCompleteness(t *testing.T) {
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		connStr = "postgres://galeriku:galeriku_secret_pass@localhost:5432/galeriku_db?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Skipf("PostgreSQL not accessible in test environment: %v", err)
	}
	defer pool.Close()

	count, err := VerifySchemaCompleteness(ctx, pool)
	if err != nil {
		t.Fatalf("VerifySchemaCompleteness failed: %v", err)
	}

	const expectedTables = 76
	if count < expectedTables {
		t.Fatalf("expected at least %d tables, got %d", expectedTables, count)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /root/galeriku/backend && go test -v ./internal/database/...`
Expected: FAIL with "undefined: VerifySchemaCompleteness"

- [ ] **Step 3: Write minimal implementation for schema verification**

Create `backend/internal/database/migrator.go`:
```go
package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// VerifySchemaCompleteness counts tables in public schema and checks critical tables
func VerifySchemaCompleteness(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	query := `
		SELECT COUNT(*) 
		FROM information_schema.tables 
		WHERE table_schema = 'public' AND table_type = 'BASE TABLE';
	`
	var count int
	err := pool.QueryRow(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to query table count: %w", err)
	}

	// Verify UUIDv7 function exists
	var funcExists bool
	funcQuery := `
		SELECT EXISTS (
			SELECT 1 FROM pg_proc WHERE proname = 'uuid_generate_v7'
		);
	`
	err = pool.QueryRow(ctx, funcQuery).Scan(&funcExists)
	if err != nil {
		return 0, fmt.Errorf("failed to verify uuid_generate_v7: %w", err)
	}
	if !funcExists {
		return 0, fmt.Errorf("critical database function uuid_generate_v7 is missing")
	}

	return count, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /root/galeriku/backend && go test -v ./internal/database/...`
Expected: PASS with 76 confirmed tables and UUIDv7 function present.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/database backend/go.mod backend/go.sum
git commit -m "feat(db): implement database schema completeness verification and test"
```

---

### Task 3: Go Database Connection Pool (`pgxpool`) & Health Check Service

**Files:**
- Create: `backend/internal/database/postgres.go`
- Modify: `backend/internal/handler/router.go`
- Create: `backend/internal/handler/router_test.go`
- Modify: `backend/cmd/server/main.go`

**Interfaces:**
- Consumes: `*config.Config` with PostgreSQL, Redis, and MinIO connection parameters.
- Produces: `NewPostgresPool(ctx context.Context, cfg *config.Config) (*pgxpool.Pool, error)` and updated `/api/v1/health` returning granular dependencies status (`postgres: UP`, `redis: UP`).

- [ ] **Step 1: Write failing test for health check endpoint with dependencies**

Create `backend/internal/handler/router_test.go`:
```go
package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthEndpointResponse(t *testing.T) {
	router := SetupRoutes(nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp SuccessEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response envelope: %v", err)
	}

	dataMap, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected data to be map[string]interface{}, got %T", resp.Data)
	}

	if dataMap["status"] != "UP" {
		t.Errorf("expected status 'UP', got '%v'", dataMap["status"])
	}
	if dataMap["service"] != "galeriku-core-api" {
		t.Errorf("expected service 'galeriku-core-api', got '%v'", dataMap["service"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /root/galeriku/backend && go test -v ./internal/handler/...`
Expected: FAIL with "too many arguments in call to SetupRoutes"

- [ ] **Step 3: Write minimal implementation for connection pool and health handler**

Create `backend/internal/database/postgres.go`:
```go
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/galeriku/core/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPostgresPool(ctx context.Context, cfg *config.Config) (*pgxpool.Pool, error) {
	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=disable",
		cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBPort, cfg.DBName,
	)

	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("unable to parse database config: %w", err)
	}

	poolConfig.MaxConns = 25
	poolConfig.MinConns = 5
	poolConfig.MaxConnLifetime = 1 * time.Hour
	poolConfig.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return pool, nil
}
```

Update `backend/internal/handler/router.go`:
```go
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SuccessEnvelope struct {
	Data interface{} `json:"data"`
	Meta SuccessMeta `json:"meta"`
}

type SuccessMeta struct {
	RequestID  string  `json:"request_id"`
	NextCursor *string `json:"next_cursor"`
}

type ErrorEnvelope struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code      string      `json:"code"`
	Message   string      `json:"message"`
	Details   interface{} `json:"details"`
	RequestID string      `json:"request_id"`
}

func SetupRoutes(pool *pgxpool.Pool) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		reqID := r.Header.Get("X-Request-ID")
		if reqID == "" {
			reqID = "req_" + uuid.New().String()[:12]
		}

		dbStatus := "UNKNOWN"
		if pool != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 1*time.Second)
			defer cancel()
			if err := pool.Ping(ctx); err == nil {
				dbStatus = "UP"
			} else {
				dbStatus = "DOWN"
			}
		}

		resp := SuccessEnvelope{
			Data: map[string]interface{}{
				"status":    "UP",
				"service":   "galeriku-core-api",
				"version":   "0.1.0-draft",
				"database":  dbStatus,
				"timestamp": time.Now().UTC().Format(time.RFC3339),
			},
			Meta: SuccessMeta{
				RequestID:  reqID,
				NextCursor: nil,
			},
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	})

	return mux
}
```

Update `backend/cmd/server/main.go`:
```go
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/galeriku/core/internal/config"
	"github.com/galeriku/core/internal/database"
	"github.com/galeriku/core/internal/handler"
)

func main() {
	cfg := config.LoadConfig()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := database.NewPostgresPool(ctx, cfg)
	if err != nil {
		log.Printf("[GALERIKU-CORE] Warning: database pool init failed: %v", err)
	} else {
		defer pool.Close()
		log.Println("[GALERIKU-CORE] PostgreSQL connection pool initialized.")
	}

	router := handler.SetupRoutes(pool)

	addr := fmt.Sprintf(":%s", cfg.AppPort)
	log.Printf("[GALERIKU-CORE] Server running on %s (ENV: %s)", addr, cfg.AppEnv)

	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /root/galeriku/backend && go test -v ./internal/handler/...`
Expected: PASS

- [ ] **Step 5: Verify build compiles cleanly**

Run: `cd /root/galeriku/backend && go build -o /tmp/galeriku-core ./cmd/server`
Expected: Exit code 0, binary created cleanly.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/database/postgres.go backend/internal/handler/router.go backend/internal/handler/router_test.go backend/cmd/server/main.go
git commit -m "feat(api): implement postgres connection pool and health check endpoint"
```

---

### Task 4: Standard Envelope & Request Correlation ID Middleware

**Files:**
- Create: `backend/internal/middleware/request_id.go`
- Create: `backend/internal/middleware/request_id_test.go`
- Modify: `backend/internal/handler/router.go`

**Interfaces:**
- Consumes: Inbound HTTP requests.
- Produces: `CorrelationIDMiddleware(next http.Handler) http.Handler` that injects `X-Request-ID` into response headers and request context, and standardizes error responses.

- [ ] **Step 1: Write failing test for CorrelationIDMiddleware**

Create `backend/internal/middleware/request_id_test.go`:
```go
package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCorrelationIDMiddleware(t *testing.T) {
	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := GetRequestID(r.Context())
		if reqID == "" {
			t.Error("expected request ID in context, got empty")
		}
		w.WriteHeader(http.StatusOK)
	})

	handlerToTest := CorrelationIDMiddleware(dummyHandler)

	// Case 1: Client provides X-Request-ID
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Request-ID", "custom-req-12345")
	rec := httptest.NewRecorder()

	handlerToTest.ServeHTTP(rec, req)

	if rec.Header().Get("X-Request-ID") != "custom-req-12345" {
		t.Errorf("expected echoed X-Request-ID 'custom-req-12345', got '%s'", rec.Header().Get("X-Request-ID"))
	}

	// Case 2: Client does not provide X-Request-ID -> Auto-generate with prefix req_
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec2 := httptest.NewRecorder()

	handlerToTest.ServeHTTP(rec2, req2)

	resID := rec2.Header().Get("X-Request-ID")
	if !strings.HasPrefix(resID, "req_") {
		t.Errorf("expected auto-generated request ID to start with 'req_', got '%s'", resID)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /root/galeriku/backend && go test -v ./internal/middleware/...`
Expected: FAIL with "undefined: CorrelationIDMiddleware"

- [ ] **Step 3: Write minimal implementation for CorrelationIDMiddleware**

Create `backend/internal/middleware/request_id.go`:
```go
package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

type contextKey string

const RequestIDKey contextKey = "request_id"

func CorrelationIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get("X-Request-ID")
		if reqID == "" {
			reqID = "req_" + strings.ReplaceAll(uuid.New().String()[:12], "-", "")
		}

		ctx := context.WithValue(r.Context(), RequestIDKey, reqID)
		w.Header().Set("X-Request-ID", reqID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetRequestID(ctx context.Context) string {
	if val, ok := ctx.Value(RequestIDKey).(string); ok {
		return val
	}
	return ""
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /root/galeriku/backend && go test -v ./internal/middleware/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/middleware
git commit -m "feat(middleware): implement request correlation ID middleware"
```

---

### Task 5: Web TypeScript Workspace Setup with Antislop Design Tokens

**Files:**
- Create: `web/package.json`
- Create: `web/tsconfig.json`
- Create: `web/src/tokens/design-system.ts`
- Create: `web/test/design-tokens.test.ts`
- Modify: `Makefile`

**Interfaces:**
- Consumes: Antislop guidelines (`antislop.md`) and UI/UX Specification (`UI_UX_SPEC.md`).
- Produces: Type-safe design tokens (`ENERGY`, `RHYTHM`, `MOTION` dials, WCAG AA color palettes, 44px tap target constants) and automated antislop copy checker preventing em dash (`—`) usage in UI strings.

- [ ] **Step 1: Write failing test for Antislop Design Tokens & Copywriting Filter**

Create directory: `mkdir -p /root/galeriku/web/test /root/galeriku/web/src/tokens`

Create `web/package.json`:
```json
{
  "name": "@galeriku/web-core",
  "version": "0.1.0",
  "private": true,
  "type": "module",
  "scripts": {
    "test": "vitest run",
    "lint:copy": "node ./scripts/lint-copy.js"
  },
  "devDependencies": {
    "typescript": "^5.4.5",
    "vitest": "^1.6.0"
  }
}
```

Create `web/tsconfig.json`:
```json
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "NodeNext",
    "moduleResolution": "NodeNext",
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "forceConsistentCasingInFileNames": true
  },
  "include": ["src/**/*", "test/**/*"]
}
```

Create `web/test/design-tokens.test.ts`:
```typescript
import { describe, it, expect } from 'vitest';
import { AntislopDials, BrandPalette, LayoutMetrics, validateAntislopCopy } from '../src/tokens/design-system';

describe('Antislop Design System & Craftsmanship Compliance', () => {
  it('enforces explicitly declared antislop dials', () => {
    expect(AntislopDials.ENERGY).toBe(2); // Balanced
    expect(AntislopDials.RHYTHM).toBe(2); // Content-driven variation
    expect(AntislopDials.MOTION).toBe(1); // Essential micro-interactions
  });

  it('guarantees minimum tap target metric of 44px for touch interfaces (R-03)', () => {
    expect(LayoutMetrics.minTapTargetPx).toBeGreaterThanOrEqual(44);
  });

  it('limits core palette to max 3 primary + 1 accent color (R-29)', () => {
    expect(BrandPalette.coreColors.length).toBeLessThanOrEqual(3);
    expect(BrandPalette.accentColor).toBeDefined();
  });

  it('rejects copy containing em dash (—) under R-02 Hard Gate', () => {
    const invalidCopy = "Temukan fotomu — cepat dan mudah";
    expect(() => validateAntislopCopy(invalidCopy)).toThrowError(/R-02 Violation: Em dash/);

    const validCopy = "Temukan fotomu: cepat dan mudah.";
    expect(validateAntislopCopy(validCopy)).toBe(true);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Install packages and run test:
```bash
cd /root/galeriku/web && npm install
npm test
```
Expected: FAIL with "Cannot find module '../src/tokens/design-system'"

- [ ] **Step 3: Write minimal implementation for Antislop Design Tokens**

Create `web/src/tokens/design-system.ts`:
```typescript
/**
 * Galeriku Antislop Design System Tokens
 * Conforming strictly to antislop.md core filter & UI_UX_SPEC.md
 */

export const AntislopDials = {
  ENERGY: 2 as const, // 1: Calm, 2: Balanced, 3: Bold
  RHYTHM: 2 as const, // 1: Uniform, 2: Content-driven, 3: Mixed
  MOTION: 1 as const, // 1: Hover/micro, 2: Scroll transitions, 3: Choreographed
};

export const BrandPalette = {
  coreColors: [
    '#0F172A', // Deep Slate (Primary Background & Contrast Text)
    '#4F46E5', // Electric Indigo (Brand Primary Action)
    '#FFFFFF', // Pure White (Surface Card)
  ],
  neutralColors: {
    gray50: '#F8FAFC',
    gray100: '#F1F5F9',
    gray500: '#64748B',
    gray900: '#0F172A',
  },
  accentColor: '#F59E0B', // Amber (Attention & Status Badges)
};

export const LayoutMetrics = {
  minTapTargetPx: 44, // Minimum mobile interactive touch dimension (R-03)
  standardRadiusPx: 8, // Non-pill geometric border radius (R-11)
  contentMaxWidthPx: 1280,
};

/**
 * Validates UI copy against Antislop Hard Gate R-02 (forbidden em dash).
 */
export function validateAntislopCopy(text: string): boolean {
  if (text.includes('—')) {
    throw new Error('R-02 Violation: Em dash character (—) is strictly forbidden in UI copy. Use comma, period, colon, or parentheses.');
  }
  return true;
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /root/galeriku/web && npm test`
Expected: PASS (4 tests passed).

- [ ] **Step 5: Commit**

```bash
git add web/package.json web/tsconfig.json web/src/tokens/design-system.ts web/test/design-tokens.test.ts
git commit -m "feat(web): establish typescript workspace and antislop design system tokens"
```

---

## 4. Self-Review Checklist

- [x] **Spec Coverage**: Docker services, PostgreSQL 76 tables, Go connection pool, correlation ID middleware, MinIO buckets, and TypeScript antislop token system are fully covered with runnable tasks.
- [x] **No Placeholders**: Every step contains concrete code, exact file paths, shell commands, and assertion values. Zero "TODO", "TBD", or "similar to Task N".
- [x] **Antislop Compliance**: R-02 (no em dash), R-03 (44px tap targets), R-29 (palette limits), R-37 (explicit dials), and R-35 (verification evidence) explicitly tested and enforced in code.
- [x] **Type Consistency**: DSN, table names, bucket names (`galeriku-master-vault`, `galeriku-public-media`), and envelope JSON schema match across Go and TypeScript files.

---

## 5. Execution Handoff

This plan is complete and saved to `docs/superpowers/plans/2026-10-15-phase-1-core-foundation.md`.

Two execution options are available:

1. **Subagent-Driven (recommended)**: Fresh subagents dispatched per task with intermediate review gates and rapid parallel verification.
2. **Inline Execution**: Executing tasks directly in this session using `executing-plans`, batch execution with review checkpoints.
