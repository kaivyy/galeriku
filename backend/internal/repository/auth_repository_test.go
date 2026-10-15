package repository_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/galeriku/core/internal/domain"
	"github.com/galeriku/core/internal/repository"
	"github.com/google/uuid"
)

func hashToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func TestAuthRepository_RefreshTokenRotation(t *testing.T) {
	pool, ctx, cancel := getTestPool(t)
	defer pool.Close()
	defer cancel()

	userRepo := repository.NewPostgresUserRepository(pool)
	authRepo := repository.NewPostgresAuthRepository(pool)

	// Create test user
	uniqueID := uuid.New().String()[:8]
	email := fmt.Sprintf("auth_user_%s@galeriku.com", uniqueID)
	user := &domain.User{EmailNormalized: &email, AccountStatus: "ACTIVE"}
	profile := &domain.UserProfile{DisplayName: "Session Tester", Locale: "id-ID"}
	creds := &domain.UserCredential{PasswordHash: "hash123", PasswordAlgo: "argon2id"}
	if err := userRepo.CreateUserWithProfileAndCredentials(ctx, user, profile, creds, "BUYER"); err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	familyID, err := uuid.NewV7()
	if err != nil {
		familyID = uuid.New()
	}

	rawToken1 := fmt.Sprintf("raw_refresh_token_1_%s", uuid.New().String())
	tokenHash1 := hashToken(rawToken1)

	tok1 := &domain.RefreshToken{
		UserID:    user.ID,
		TokenHash: tokenHash1,
		FamilyID:  familyID,
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour),
	}

	// 1. Create initial refresh token
	if err := authRepo.CreateRefreshToken(ctx, tok1); err != nil {
		t.Fatalf("failed to create initial refresh token: %v", err)
	}

	// 2. Fetch by hash
	fetched1, err := authRepo.GetRefreshTokenByHash(ctx, tokenHash1)
	if err != nil {
		t.Fatalf("failed to get token by hash: %v", err)
	}
	if fetched1.IsRevoked {
		t.Error("expected fresh token to not be revoked")
	}

	// 3. Rotate token (Token 1 -> Token 2)
	rawToken2 := fmt.Sprintf("raw_refresh_token_2_%s", uuid.New().String())
	tokenHash2 := hashToken(rawToken2)
	tok2 := &domain.RefreshToken{
		UserID:    user.ID,
		TokenHash: tokenHash2,
		FamilyID:  familyID,
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour),
	}

	if err := authRepo.RotateRefreshToken(ctx, fetched1.ID, tok2); err != nil {
		t.Fatalf("failed to rotate refresh token: %v", err)
	}

	// Verify old token is now marked revoked
	oldToken, err := authRepo.GetRefreshTokenByHash(ctx, tokenHash1)
	if err != nil {
		t.Fatalf("failed to get old token: %v", err)
	}
	if !oldToken.IsRevoked {
		t.Error("expected old token to be marked revoked")
	}
	if oldToken.RevokedReason == nil || *oldToken.RevokedReason != "ROTATED" {
		t.Errorf("expected revoked_reason 'ROTATED', got: %v", oldToken.RevokedReason)
	}

	// Verify new token is active
	newToken, err := authRepo.GetRefreshTokenByHash(ctx, tokenHash2)
	if err != nil {
		t.Fatalf("failed to get new token: %v", err)
	}
	if newToken.IsRevoked {
		t.Error("expected successor token to be active")
	}

	// 4. Token Reuse Detection: Revoke entire family
	if err := authRepo.RevokeFamily(ctx, familyID, "REUSE_DETECTED"); err != nil {
		t.Fatalf("failed to revoke token family: %v", err)
	}

	// Verify successor token is now revoked too
	newTokenAfterRevocation, err := authRepo.GetRefreshTokenByHash(ctx, tokenHash2)
	if err != nil {
		t.Fatalf("failed to fetch successor token after family revocation: %v", err)
	}
	if !newTokenAfterRevocation.IsRevoked {
		t.Error("expected successor token to be revoked after family revocation")
	}
}

func TestAuthRepository_OTPChallenges(t *testing.T) {
	pool, ctx, cancel := getTestPool(t)
	defer pool.Close()
	defer cancel()

	authRepo := repository.NewPostgresAuthRepository(pool)

	phone := fmt.Sprintf("+62813%08d", time.Now().UnixNano()%100000000)
	codeHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	challenge := &domain.OTPChallenge{
		PhoneE164:   &phone,
		Purpose:     "PHONE_VERIFICATION",
		CodeHash:    codeHash,
		MaxAttempts: 3,
		Status:      "PENDING",
		ExpiresAt:   time.Now().Add(3 * time.Minute),
	}

	// 1. Create OTP challenge
	if err := authRepo.CreateOTPChallenge(ctx, challenge); err != nil {
		t.Fatalf("failed to create OTP challenge: %v", err)
	}
	if challenge.ID == uuid.Nil {
		t.Fatal("expected challenge.ID to be populated")
	}

	// 2. Fetch by ID
	fetched, err := authRepo.GetOTPChallengeByID(ctx, challenge.ID)
	if err != nil {
		t.Fatalf("failed to get challenge by ID: %v", err)
	}
	if fetched.Status != "PENDING" {
		t.Errorf("expected status 'PENDING', got %q", fetched.Status)
	}

	// 3. Increment attempts
	newAttempts, err := authRepo.IncrementOTPAttempts(ctx, challenge.ID)
	if err != nil {
		t.Fatalf("failed to increment attempts: %v", err)
	}
	if newAttempts != 1 {
		t.Errorf("expected 1 attempt, got %d", newAttempts)
	}

	// 4. Mark verified
	if err := authRepo.MarkOTPVerified(ctx, challenge.ID); err != nil {
		t.Fatalf("failed to mark challenge verified: %v", err)
	}

	verifiedChallenge, err := authRepo.GetOTPChallengeByID(ctx, challenge.ID)
	if err != nil {
		t.Fatalf("failed to get verified challenge: %v", err)
	}
	if verifiedChallenge.Status != "VERIFIED" {
		t.Errorf("expected status 'VERIFIED', got %q", verifiedChallenge.Status)
	}
	if verifiedChallenge.VerifiedAt == nil {
		t.Error("expected verified_at timestamp to be non-nil")
	}
}
