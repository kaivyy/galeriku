package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/galeriku/core/internal/domain"
	"github.com/galeriku/core/internal/repository"
	"github.com/galeriku/core/internal/security"
	"github.com/galeriku/core/internal/usecase"
	"github.com/google/uuid"
)

// --- Mocks ---

type mockUserRepo struct {
	users        map[string]*domain.User
	creds        map[uuid.UUID]*domain.UserCredential
	profiles     map[uuid.UUID]*domain.UserProfile
	capabilities map[uuid.UUID][]string
	memberships  map[uuid.UUID][]domain.HostMembershipSummary
}

func newMockUserRepo() *mockUserRepo {
	return &mockUserRepo{
		users:        make(map[string]*domain.User),
		creds:        make(map[uuid.UUID]*domain.UserCredential),
		profiles:     make(map[uuid.UUID]*domain.UserProfile),
		capabilities: make(map[uuid.UUID][]string),
		memberships:  make(map[uuid.UUID][]domain.HostMembershipSummary),
	}
}

func (m *mockUserRepo) CreateUserWithProfileAndCredentials(ctx context.Context, user *domain.User, profile *domain.UserProfile, creds *domain.UserCredential, capabilityCode string) error {
	if user.EmailNormalized != nil {
		if _, exists := m.users[*user.EmailNormalized]; exists {
			return repository.ErrDuplicateAccount
		}
	}
	user.ID = uuid.New()
	user.CreatedAt = time.Now()
	user.UpdatedAt = time.Now()

	profile.ID = uuid.New()
	profile.UserID = user.ID
	profile.CreatedAt = time.Now()

	creds.ID = uuid.New()
	creds.UserID = user.ID
	creds.CreatedAt = time.Now()

	if user.EmailNormalized != nil {
		m.users[*user.EmailNormalized] = user
	}
	if user.PhoneE164 != nil {
		m.users[*user.PhoneE164] = user
	}
	m.creds[user.ID] = creds
	m.profiles[user.ID] = profile
	m.capabilities[user.ID] = []string{capabilityCode}

	return nil
}

func (m *mockUserRepo) GetByIdentifier(ctx context.Context, identifier string) (*domain.User, *domain.UserCredential, error) {
	user, exists := m.users[identifier]
	if !exists {
		return nil, nil, repository.ErrUserNotFound
	}
	return user, m.creds[user.ID], nil
}

func (m *mockUserRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	for _, u := range m.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, repository.ErrUserNotFound
}

func (m *mockUserRepo) GetUserProfile(ctx context.Context, userID uuid.UUID) (*domain.UserProfile, error) {
	p, ok := m.profiles[userID]
	if !ok {
		return nil, repository.ErrUserNotFound
	}
	return p, nil
}

func (m *mockUserRepo) GetUserCapabilities(ctx context.Context, userID uuid.UUID) ([]string, error) {
	return m.capabilities[userID], nil
}

func (m *mockUserRepo) GetUserHostMemberships(ctx context.Context, userID uuid.UUID) ([]domain.HostMembershipSummary, error) {
	return m.memberships[userID], nil
}

func (m *mockUserRepo) UpdateFailedAttempts(ctx context.Context, userID uuid.UUID, failedAttempts int, lockedUntil *time.Time) error {
	if c, ok := m.creds[userID]; ok {
		c.FailedAttempts = failedAttempts
		c.LockedUntil = lockedUntil
	}
	return nil
}

func (m *mockUserRepo) ResetFailedAttempts(ctx context.Context, userID uuid.UUID) error {
	if c, ok := m.creds[userID]; ok {
		c.FailedAttempts = 0
		c.LockedUntil = nil
	}
	return nil
}

type mockAuthRepo struct {
	tokens     map[string]*domain.RefreshToken
	challenges map[uuid.UUID]*domain.OTPChallenge
}

func newMockAuthRepo() *mockAuthRepo {
	return &mockAuthRepo{
		tokens:     make(map[string]*domain.RefreshToken),
		challenges: make(map[uuid.UUID]*domain.OTPChallenge),
	}
}

func (m *mockAuthRepo) CreateRefreshToken(ctx context.Context, token *domain.RefreshToken) error {
	token.ID = uuid.New()
	token.CreatedAt = time.Now()
	m.tokens[token.TokenHash] = token
	return nil
}

func (m *mockAuthRepo) GetRefreshTokenByHash(ctx context.Context, tokenHash string) (*domain.RefreshToken, error) {
	t, ok := m.tokens[tokenHash]
	if !ok {
		return nil, repository.ErrTokenNotFound
	}
	return t, nil
}

func (m *mockAuthRepo) RotateRefreshToken(ctx context.Context, oldTokenID uuid.UUID, newToken *domain.RefreshToken) error {
	for _, t := range m.tokens {
		if t.ID == oldTokenID {
			if t.IsRevoked {
				return repository.ErrTokenAlreadyRevoked
			}
			t.IsRevoked = true
			reason := "ROTATED"
			t.RevokedReason = &reason
			return m.CreateRefreshToken(ctx, newToken)
		}
	}
	return repository.ErrTokenNotFound
}

func (m *mockAuthRepo) RevokeFamily(ctx context.Context, familyID uuid.UUID, reason string) error {
	for _, t := range m.tokens {
		if t.FamilyID == familyID {
			t.IsRevoked = true
			r := reason
			t.RevokedReason = &r
		}
	}
	return nil
}

func (m *mockAuthRepo) RevokeToken(ctx context.Context, tokenID uuid.UUID, reason string) error {
	for _, t := range m.tokens {
		if t.ID == tokenID {
			t.IsRevoked = true
			r := reason
			t.RevokedReason = &r
		}
	}
	return nil
}

func (m *mockAuthRepo) CreateOTPChallenge(ctx context.Context, challenge *domain.OTPChallenge) error {
	challenge.ID = uuid.New()
	challenge.CreatedAt = time.Now()
	m.challenges[challenge.ID] = challenge
	return nil
}

func (m *mockAuthRepo) GetOTPChallengeByID(ctx context.Context, id uuid.UUID) (*domain.OTPChallenge, error) {
	c, ok := m.challenges[id]
	if !ok {
		return nil, repository.ErrChallengeNotFound
	}
	return c, nil
}

func (m *mockAuthRepo) IncrementOTPAttempts(ctx context.Context, id uuid.UUID) (int, error) {
	c, ok := m.challenges[id]
	if !ok {
		return 0, repository.ErrChallengeNotFound
	}
	c.AttemptCount++
	return c.AttemptCount, nil
}

func (m *mockAuthRepo) MarkOTPVerified(ctx context.Context, id uuid.UUID) error {
	c, ok := m.challenges[id]
	if !ok || c.Status != "PENDING" {
		return repository.ErrChallengeNotFound
	}
	c.Status = "VERIFIED"
	now := time.Now()
	c.VerifiedAt = &now
	return nil
}

func (m *mockAuthRepo) MarkOTPStatus(ctx context.Context, id uuid.UUID, status string) error {
	c, ok := m.challenges[id]
	if !ok {
		return repository.ErrChallengeNotFound
	}
	c.Status = status
	return nil
}

type mockRateLimiter struct {
	allowedOTPSend bool
	allowedLogin   bool
}

func (m *mockRateLimiter) AllowOTPSend(ctx context.Context, identifier string) (security.RateLimitResult, error) {
	if !m.allowedOTPSend {
		return security.RateLimitResult{Allowed: false, RetryAfter: 60 * time.Second, Reason: "BURST_EXCEEDED"}, nil
	}
	return security.RateLimitResult{Allowed: true}, nil
}

func (m *mockRateLimiter) AllowLogin(ctx context.Context, identifier string) (security.RateLimitResult, error) {
	if !m.allowedLogin {
		return security.RateLimitResult{Allowed: false, RetryAfter: 60 * time.Second, Reason: "BURST_EXCEEDED"}, nil
	}
	return security.RateLimitResult{Allowed: true}, nil
}

func (m *mockRateLimiter) ClearCooldown(ctx context.Context, identifier string) error { return nil }
func (m *mockRateLimiter) Reset(ctx context.Context, identifier string) error         { return nil }

type mockNotifier struct {
	lastTarget string
	lastCode   string
}

func (m *mockNotifier) NotifyOTP(ctx context.Context, target, code, purpose, channel string) error {
	m.lastTarget = target
	m.lastCode = code
	return nil
}

// --- Unit Tests ---

const (
	testJWTSecret = "unit_test_jwt_secret_32_bytes_long_12345!"
	testOTPSecret = "unit_test_otp_secret_32_bytes_long_67890!"
)

func TestAuthUsecase_Register_Success(t *testing.T) {
	userRepo := newMockUserRepo()
	authRepo := newMockAuthRepo()
	limiter := &mockRateLimiter{allowedLogin: true, allowedOTPSend: true}

	uc := usecase.NewAuthUsecase(userRepo, authRepo, limiter, testJWTSecret, testOTPSecret, 15*time.Minute, 30*24*time.Hour, nil)

	req := domain.RegisterRequest{
		Email:       "newuser@galeriku.com",
		PhoneE164:   "+628123456789",
		Password:    "ValidPassword123!",
		DisplayName: "New User",
	}

	resp, err := uc.Register(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ID == "" {
		t.Error("expected non-empty user ID")
	}
	if resp.DisplayName != "New User" {
		t.Errorf("expected display_name 'New User', got: %s", resp.DisplayName)
	}
	if len(resp.Capabilities) != 1 || resp.Capabilities[0] != "BUYER" {
		t.Errorf("expected capabilities [BUYER], got: %v", resp.Capabilities)
	}
}

func TestAuthUsecase_Register_WeakPassword(t *testing.T) {
	userRepo := newMockUserRepo()
	authRepo := newMockAuthRepo()
	limiter := &mockRateLimiter{allowedLogin: true, allowedOTPSend: true}
	uc := usecase.NewAuthUsecase(userRepo, authRepo, limiter, testJWTSecret, testOTPSecret, 15*time.Minute, 30*24*time.Hour, nil)

	req := domain.RegisterRequest{
		Email:       "user@galeriku.com",
		Password:    "weak",
		DisplayName: "User",
	}

	_, err := uc.Register(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for weak password, got nil")
	}
	if !errors.Is(err, security.ErrPasswordComplexity) {
		t.Errorf("expected ErrPasswordComplexity, got: %v", err)
	}
}

func TestAuthUsecase_Login_Success(t *testing.T) {
	userRepo := newMockUserRepo()
	authRepo := newMockAuthRepo()
	limiter := &mockRateLimiter{allowedLogin: true, allowedOTPSend: true}
	uc := usecase.NewAuthUsecase(userRepo, authRepo, limiter, testJWTSecret, testOTPSecret, 15*time.Minute, 30*24*time.Hour, nil)

	// Register user first
	_, err := uc.Register(context.Background(), domain.RegisterRequest{
		Email:       "loginuser@galeriku.com",
		Password:    "SecretPass123!",
		DisplayName: "Login User",
	})
	if err != nil {
		t.Fatalf("failed to register user: %v", err)
	}

	// Login
	loginResp, err := uc.Login(context.Background(), domain.LoginRequest{
		Identifier: "loginuser@galeriku.com",
		Password:   "SecretPass123!",
	}, "TestAgent", "127.0.0.1")

	if err != nil {
		t.Fatalf("unexpected login error: %v", err)
	}

	if loginResp.AccessToken == "" {
		t.Error("expected access_token to be non-empty")
	}
	if loginResp.RefreshToken == "" {
		t.Error("expected refresh_token to be non-empty")
	}
	if loginResp.TokenType != "Bearer" {
		t.Errorf("expected token_type 'Bearer', got: %s", loginResp.TokenType)
	}
	if loginResp.User.DisplayName != "Login User" {
		t.Errorf("expected user display_name 'Login User', got: %s", loginResp.User.DisplayName)
	}
}

func TestAuthUsecase_Login_RateLimited(t *testing.T) {
	userRepo := newMockUserRepo()
	authRepo := newMockAuthRepo()
	// Disallow login at limiter
	limiter := &mockRateLimiter{allowedLogin: false, allowedOTPSend: true}
	uc := usecase.NewAuthUsecase(userRepo, authRepo, limiter, testJWTSecret, testOTPSecret, 15*time.Minute, 30*24*time.Hour, nil)

	_, err := uc.Login(context.Background(), domain.LoginRequest{
		Identifier: "throttled@galeriku.com",
		Password:   "AnyPassword123!",
	}, "TestAgent", "127.0.0.1")

	if err == nil {
		t.Fatal("expected error for throttled login, got nil")
	}
	if !errors.Is(err, usecase.ErrTooManyRequests) {
		t.Errorf("expected ErrTooManyRequests, got: %v", err)
	}
}

func TestAuthUsecase_RefreshToken_RotationAndReuse(t *testing.T) {
	userRepo := newMockUserRepo()
	authRepo := newMockAuthRepo()
	limiter := &mockRateLimiter{allowedLogin: true, allowedOTPSend: true}
	uc := usecase.NewAuthUsecase(userRepo, authRepo, limiter, testJWTSecret, testOTPSecret, 15*time.Minute, 30*24*time.Hour, nil)

	// Register and login
	_, _ = uc.Register(context.Background(), domain.RegisterRequest{
		Email:       "rtr@galeriku.com",
		Password:    "SecretPass123!",
		DisplayName: "RTR User",
	})
	loginResp, _ := uc.Login(context.Background(), domain.LoginRequest{
		Identifier: "rtr@galeriku.com",
		Password:   "SecretPass123!",
	}, "TestAgent", "127.0.0.1")

	token1 := loginResp.RefreshToken

	// 1. First refresh -> Success
	refResp, err := uc.RefreshToken(context.Background(), domain.RefreshTokenRequest{RefreshToken: token1}, "TestAgent", "127.0.0.1")
	if err != nil {
		t.Fatalf("unexpected refresh error: %v", err)
	}
	token2 := refResp.RefreshToken
	if token2 == "" || token2 == token1 {
		t.Errorf("expected new successor refresh token, got: %s", token2)
	}

	// 2. Reuse token1 -> Expect ErrTokenCompromised
	_, err = uc.RefreshToken(context.Background(), domain.RefreshTokenRequest{RefreshToken: token1}, "TestAgent", "127.0.0.1")
	if err == nil {
		t.Fatal("expected error on token reuse, got nil")
	}
	if !errors.Is(err, usecase.ErrTokenCompromised) {
		t.Errorf("expected ErrTokenCompromised, got: %v", err)
	}

	// 3. Successor token2 must now also be rejected
	_, err = uc.RefreshToken(context.Background(), domain.RefreshTokenRequest{RefreshToken: token2}, "TestAgent", "127.0.0.1")
	if err == nil {
		t.Fatal("expected successor token to be revoked following reuse attack, got nil")
	}
	if !errors.Is(err, usecase.ErrTokenCompromised) {
		t.Errorf("expected ErrTokenCompromised for successor, got: %v", err)
	}
}

func TestAuthUsecase_OTP_SendAndVerify(t *testing.T) {
	userRepo := newMockUserRepo()
	authRepo := newMockAuthRepo()
	limiter := &mockRateLimiter{allowedLogin: true, allowedOTPSend: true}
	notifier := &mockNotifier{}

	uc := usecase.NewAuthUsecase(userRepo, authRepo, limiter, testJWTSecret, testOTPSecret, 15*time.Minute, 30*24*time.Hour, nil)
	if setter, ok := uc.(interface{ SetOTPNotifier(usecase.OTPNotifier) }); ok {
		setter.SetOTPNotifier(notifier)
	}

	// 1. Send OTP
	sendResp, err := uc.SendOTP(context.Background(), domain.SendOTPRequest{
		PhoneE164: "+6281234567890",
		Purpose:   "PHONE_VERIFICATION",
	})
	if err != nil {
		t.Fatalf("unexpected SendOTP error: %v", err)
	}
	if sendResp.ChallengeID == "" {
		t.Fatal("expected non-empty challenge ID")
	}
	if notifier.lastCode == "" {
		t.Fatal("expected notifier to receive generated OTP code")
	}

	// 2. Verify with wrong code -> fail
	_, err = uc.VerifyOTP(context.Background(), domain.VerifyOTPRequest{
		ChallengeID: sendResp.ChallengeID,
		OTPCode:     "000000",
	})
	if err == nil {
		t.Fatal("expected error for wrong code, got nil")
	}
	if !errors.Is(err, usecase.ErrIncorrectOTP) {
		t.Errorf("expected ErrIncorrectOTP, got: %v", err)
	}

	// 3. Verify with correct code -> success
	verifyResp, err := uc.VerifyOTP(context.Background(), domain.VerifyOTPRequest{
		ChallengeID: sendResp.ChallengeID,
		OTPCode:     notifier.lastCode,
	})
	if err != nil {
		t.Fatalf("unexpected VerifyOTP error: %v", err)
	}
	if !verifyResp.Verified {
		t.Error("expected verified: true")
	}
	if verifyResp.VerificationToken == "" {
		t.Error("expected non-empty verification_token")
	}

	// 4. Replay verification -> fail
	_, err = uc.VerifyOTP(context.Background(), domain.VerifyOTPRequest{
		ChallengeID: sendResp.ChallengeID,
		OTPCode:     notifier.lastCode,
	})
	if err == nil {
		t.Fatal("expected error for replayed OTP verification, got nil")
	}
	if !errors.Is(err, usecase.ErrInvalidOTP) {
		t.Errorf("expected ErrInvalidOTP for already verified challenge, got: %v", err)
	}
}
