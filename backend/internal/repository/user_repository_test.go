package repository_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/galeriku/core/internal/domain"
	"github.com/galeriku/core/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func getTestPool(t *testing.T) (*pgxpool.Pool, context.Context, context.CancelFunc) {
	if testing.Short() {
		t.Skip("skipping live database test in short mode")
	}

	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
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
		dbName := os.Getenv("DB_NAME")
		if dbName == "" {
			dbName = "galeriku_db"
		}
		sslMode := os.Getenv("DB_SSLMODE")
		if sslMode == "" {
			sslMode = "disable"
		}
		connStr = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s", user, pass, host, port, dbName, sslMode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		cancel()
		t.Skipf("PostgreSQL not accessible in test environment: %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		cancel()
		t.Skipf("PostgreSQL ping failed: %v", err)
	}

	return pool, ctx, cancel
}

func TestUserRepository_CreateAndGetByIdentifier(t *testing.T) {
	pool, ctx, cancel := getTestPool(t)
	defer pool.Close()
	defer cancel()

	repo := repository.NewPostgresUserRepository(pool)

	uniqueID := uuid.New().String()[:8]
	email := fmt.Sprintf("testuser_%s@galeriku.com", uniqueID)
	phone := fmt.Sprintf("+62812%08d", time.Now().UnixNano()%100000000)
	displayName := "Budi Santoso"
	passwordHash := "$argon2id$v=19$m=65536,t=3,p=4$dummy_salt$dummy_hash"

	user := &domain.User{
		EmailNormalized: &email,
		PhoneE164:       &phone,
		AccountStatus:   "ACTIVE",
	}
	profile := &domain.UserProfile{
		DisplayName: displayName,
		Locale:      "id-ID",
	}
	creds := &domain.UserCredential{
		PasswordHash: passwordHash,
		PasswordAlgo: "argon2id",
	}

	// 1. Create user with profile, creds, and BUYER capability
	err := repo.CreateUserWithProfileAndCredentials(ctx, user, profile, creds, "BUYER")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	if user.ID == uuid.Nil {
		t.Fatal("expected user.ID to be populated with UUID")
	}

	// 2. Lookup by email
	fetchedUser, fetchedCreds, err := repo.GetByIdentifier(ctx, email)
	if err != nil {
		t.Fatalf("failed to get user by email: %v", err)
	}
	if fetchedUser.ID != user.ID {
		t.Errorf("expected user ID %v, got %v", user.ID, fetchedUser.ID)
	}
	if fetchedCreds.PasswordHash != passwordHash {
		t.Errorf("expected password hash %q, got %q", passwordHash, fetchedCreds.PasswordHash)
	}

	// 3. Lookup by phone
	fetchedUserByPhone, _, err := repo.GetByIdentifier(ctx, phone)
	if err != nil {
		t.Fatalf("failed to get user by phone: %v", err)
	}
	if fetchedUserByPhone.ID != user.ID {
		t.Errorf("expected user ID %v, got %v", user.ID, fetchedUserByPhone.ID)
	}

	// 4. Verify user capabilities
	caps, err := repo.GetUserCapabilities(ctx, user.ID)
	if err != nil {
		t.Fatalf("failed to get user capabilities: %v", err)
	}
	if len(caps) != 1 || caps[0] != "BUYER" {
		t.Errorf("expected capabilities [BUYER], got: %v", caps)
	}

	// 5. Duplicate account conflict
	dupUser := &domain.User{
		EmailNormalized: &email,
		AccountStatus:   "ACTIVE",
	}
	dupProfile := &domain.UserProfile{
		DisplayName: "Duplicate",
		Locale:      "id-ID",
	}
	dupCreds := &domain.UserCredential{
		PasswordHash: passwordHash,
		PasswordAlgo: "argon2id",
	}
	err = repo.CreateUserWithProfileAndCredentials(ctx, dupUser, dupProfile, dupCreds, "BUYER")
	if err == nil {
		t.Fatal("expected duplicate account error, got nil")
	}
	if err != repository.ErrDuplicateAccount {
		t.Errorf("expected ErrDuplicateAccount, got: %v", err)
	}
}
