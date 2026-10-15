package usecase

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/galeriku/core/internal/audit"
	"github.com/galeriku/core/internal/domain"
	"github.com/galeriku/core/internal/repository"
	"github.com/galeriku/core/internal/security"
	"github.com/google/uuid"
)

var (
	ErrInvalidInput        = errors.New("invalid request payload")
	ErrInvalidCredentials  = errors.New("invalid identifier or password")
	ErrAccountInactive     = errors.New("account is suspended or inactive")
	ErrTooManyRequests     = errors.New("rate limit exceeded, please retry later")
	ErrInvalidToken        = errors.New("invalid or malformed refresh token")
	ErrTokenExpired        = errors.New("refresh token has expired")
	ErrTokenCompromised    = errors.New("token reuse detected, session terminated")
	ErrInvalidOTP          = errors.New("invalid or expired OTP challenge")
	ErrOTPExpired          = errors.New("OTP challenge has expired")
	ErrMaxAttemptsExceeded = errors.New("maximum OTP verification attempts exceeded")
	ErrIncorrectOTP        = errors.New("incorrect OTP verification code")
)

type AuthUsecase interface {
	Register(ctx context.Context, req domain.RegisterRequest) (*domain.RegisterResponse, error)
	Login(ctx context.Context, req domain.LoginRequest, userAgent, ipAddress string) (*domain.LoginResponse, error)
	RefreshToken(ctx context.Context, req domain.RefreshTokenRequest, userAgent, ipAddress string) (*domain.RefreshTokenResponse, error)
	Logout(ctx context.Context, req domain.LogoutRequest) error
	SendOTP(ctx context.Context, req domain.SendOTPRequest) (*domain.SendOTPResponse, error)
	VerifyOTP(ctx context.Context, req domain.VerifyOTPRequest) (*domain.VerifyOTPResponse, error)
	GetCurrentUser(ctx context.Context, userID uuid.UUID) (*domain.CurrentUserResponse, error)
}

// OTPNotifier defines the interface for delivering generated OTP codes.
type OTPNotifier interface {
	NotifyOTP(ctx context.Context, target, code, purpose, channel string) error
}

type NoopOTPNotifier struct{}

func (n *NoopOTPNotifier) NotifyOTP(ctx context.Context, target, code, purpose, channel string) error {
	return nil
}

type authUsecase struct {
	userRepo   repository.UserRepository
	authRepo   repository.AuthRepository
	limiter    security.RateLimiter
	notifier   OTPNotifier
	auditor    audit.Logger
	jwtSecret  []byte
	otpSecret  []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func NewAuthUsecase(
	userRepo repository.UserRepository,
	authRepo repository.AuthRepository,
	limiter security.RateLimiter,
	jwtSecret string,
	otpSecret string,
	accessTTL time.Duration,
	refreshTTL time.Duration,
	auditor audit.Logger,
) AuthUsecase {
	if accessTTL <= 0 {
		accessTTL = 15 * time.Minute
	}
	if refreshTTL <= 0 {
		refreshTTL = 30 * 24 * time.Hour
	}
	return &authUsecase{
		userRepo:   userRepo,
		authRepo:   authRepo,
		limiter:    limiter,
		notifier:   &NoopOTPNotifier{},
		auditor:    auditor,
		jwtSecret:  []byte(jwtSecret),
		otpSecret:  []byte(otpSecret),
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
	}
}

// recordAudit writes one audit entry when a logger is configured. Audit failures never
// block the caller: the security decision has already been made, and losing the mutation
// would be worse than losing its log line. Failures are surfaced to the process log.
func (u *authUsecase) recordAudit(ctx context.Context, entry audit.Entry) {
	if u.auditor == nil {
		return
	}
	if err := u.auditor.Log(ctx, entry); err != nil {
		log.Printf("[AUDIT] failed to record %s: %v", entry.ActionCode, err)
	}
}

// SetOTPNotifier sets a custom OTP delivery notifier (e.g. for testing or production gateway).
func (u *authUsecase) SetOTPNotifier(notifier OTPNotifier) {
	if notifier != nil {
		u.notifier = notifier
	}
}

// GenerateCSPRNGRefreshToken creates a 256-bit cryptographically secure token and its SHA-256 digest.
func GenerateCSPRNGRefreshToken() (rawToken string, tokenHash string, err error) {
	bytes := make([]byte, 32) // 256 bits of entropy
	if _, err := rand.Read(bytes); err != nil {
		return "", "", fmt.Errorf("failed to sample random bytes: %w", err)
	}
	rawToken = base64.RawURLEncoding.EncodeToString(bytes)
	digest := sha256.Sum256([]byte(rawToken))
	tokenHash = hex.EncodeToString(digest[:])
	return rawToken, tokenHash, nil
}

func (u *authUsecase) Register(ctx context.Context, req domain.RegisterRequest) (*domain.RegisterResponse, error) {
	email := strings.TrimSpace(req.Email)
	phone := strings.TrimSpace(req.PhoneE164)
	displayName := strings.TrimSpace(req.DisplayName)

	if email == "" && phone == "" {
		return nil, fmt.Errorf("%w: either email or phone_e164 must be provided", ErrInvalidInput)
	}
	if displayName == "" {
		return nil, fmt.Errorf("%w: display_name is required", ErrInvalidInput)
	}
	if err := security.ValidatePasswordComplexity(req.Password); err != nil {
		return nil, err
	}

	hashedPassword, err := security.HashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	var emailPtr, phonePtr *string
	if email != "" {
		normEmail := strings.ToLower(email)
		emailPtr = &normEmail
	}
	if phone != "" {
		phonePtr = &phone
	}

	user := &domain.User{
		EmailNormalized: emailPtr,
		PhoneE164:       phonePtr,
		AccountStatus:   "ACTIVE",
	}
	profile := &domain.UserProfile{
		DisplayName: displayName,
		Locale:      "id-ID",
	}
	creds := &domain.UserCredential{
		PasswordHash: hashedPassword,
		PasswordAlgo: "argon2id",
	}

	// Default capability is BUYER per PRD and implementation plan
	if err := u.userRepo.CreateUserWithProfileAndCredentials(ctx, user, profile, creds, "BUYER"); err != nil {
		return nil, err
	}

	resp := &domain.RegisterResponse{
		ID:           user.ID.String(),
		DisplayName:  profile.DisplayName,
		Capabilities: []string{"BUYER"},
	}
	if user.EmailNormalized != nil {
		resp.EmailNormalized = *user.EmailNormalized
	}
	if user.PhoneE164 != nil {
		resp.PhoneE164 = *user.PhoneE164
	}

	return resp, nil
}

func (u *authUsecase) Login(ctx context.Context, req domain.LoginRequest, userAgent, ipAddress string) (*domain.LoginResponse, error) {
	identifier := strings.TrimSpace(req.Identifier)
	if identifier == "" || req.Password == "" {
		return nil, fmt.Errorf("%w: identifier and password are required", ErrInvalidInput)
	}

	// 1. Mandatory Login Throttling: Run rate limiter BEFORE expensive Argon2id verification
	if u.limiter != nil {
		res, err := u.limiter.AllowLogin(ctx, identifier)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate rate limit: %w", err)
		}
		if !res.Allowed {
			return nil, ErrTooManyRequests
		}
	}

	// 2. Lookup user and credentials
	user, creds, err := u.userRepo.GetByIdentifier(ctx, identifier)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			// Mitigate account enumeration timing attacks by executing full Argon2id verification
			const dummyHash = "$argon2id$v=19$m=65536,t=3,p=4$ZHVtbXlzYWx0MTIzNA$ZHVtbXloYXNoMTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM"
			_, _ = security.VerifyPassword(req.Password, dummyHash)
			u.recordAudit(ctx, audit.Entry{
				ActionCode: "AUTH_LOGIN_FAILED",
				TargetType: "user",
				ReasonCode: ptrString("UNKNOWN_IDENTIFIER"),
				Metadata:   map[string]interface{}{"user_agent": userAgent, "ip_address": ipAddress},
			})
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("failed to query user: %w", err)
	}

	if user.AccountStatus != "ACTIVE" {
		return nil, ErrAccountInactive
	}

	// Check temporary lockout if present
	if creds.LockedUntil != nil && creds.LockedUntil.After(time.Now()) {
		return nil, ErrTooManyRequests
	}

	// 3. Verify password via Argon2id
	match, err := security.VerifyPassword(req.Password, creds.PasswordHash)
	if err != nil || !match {
		newFailed := creds.FailedAttempts + 1
		var lockedUntil *time.Time
		if newFailed >= 5 {
			tLock := time.Now().Add(15 * time.Minute)
			lockedUntil = &tLock
		}
		_ = u.userRepo.UpdateFailedAttempts(ctx, user.ID, newFailed, lockedUntil)
		u.recordAudit(ctx, audit.Entry{
			ActorUserID: &user.ID,
			ActionCode:  "AUTH_LOGIN_FAILED",
			TargetType:  "user",
			TargetID:    &user.ID,
			ReasonCode:  ptrString("INVALID_PASSWORD"),
			Metadata:    map[string]interface{}{"failed_attempts": newFailed, "ip_address": ipAddress},
		})
		return nil, ErrInvalidCredentials
	}

	// Reset failed attempts on success
	if creds.FailedAttempts > 0 {
		_ = u.userRepo.ResetFailedAttempts(ctx, user.ID)
	}

	// 4. Fetch capabilities and profile
	caps, err := u.userRepo.GetUserCapabilities(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user capabilities: %w", err)
	}

	profile, err := u.userRepo.GetUserProfile(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user profile: %w", err)
	}

	// 5. Mint access token
	emailVal := ""
	if user.EmailNormalized != nil {
		emailVal = *user.EmailNormalized
	}
	accessTok, err := security.GenerateAccessToken(u.jwtSecret, user.ID.String(), emailVal, caps, u.accessTTL)
	if err != nil {
		return nil, fmt.Errorf("failed to mint access token: %w", err)
	}

	// 6. Generate stateful refresh token with 256 bits of entropy
	rawRefreshTok, tokenHash, err := GenerateCSPRNGRefreshToken()
	if err != nil {
		return nil, err
	}

	familyID, err := uuid.NewV7()
	if err != nil {
		familyID = uuid.New()
	}

	refreshTokenObj := &domain.RefreshToken{
		UserID:    user.ID,
		TokenHash: tokenHash,
		FamilyID:  familyID,
		ExpiresAt: time.Now().Add(u.refreshTTL),
		UserAgent: &userAgent,
		IPAddress: &ipAddress,
	}

	if err := u.authRepo.CreateRefreshToken(ctx, refreshTokenObj); err != nil {
		return nil, fmt.Errorf("failed to persist refresh token: %w", err)
	}

	return &domain.LoginResponse{
		AccessToken:  accessTok,
		RefreshToken: rawRefreshTok,
		TokenType:    "Bearer",
		ExpiresIn:    int64(u.accessTTL.Seconds()),
		User: domain.UserSummary{
			ID:           user.ID.String(),
			DisplayName:  profile.DisplayName,
			Capabilities: caps,
		},
	}, nil
}

func (u *authUsecase) RefreshToken(ctx context.Context, req domain.RefreshTokenRequest, userAgent, ipAddress string) (*domain.RefreshTokenResponse, error) {
	rawToken := strings.TrimSpace(req.RefreshToken)
	if rawToken == "" {
		return nil, fmt.Errorf("%w: refresh_token is required", ErrInvalidInput)
	}

	tokenDigest := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(tokenDigest[:])

	storedToken, err := u.authRepo.GetRefreshTokenByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, repository.ErrTokenNotFound) {
			return nil, ErrInvalidToken
		}
		return nil, fmt.Errorf("failed to lookup refresh token: %w", err)
	}

	// Token Reuse Detection
	if storedToken.IsRevoked {
		_ = u.authRepo.RevokeFamily(ctx, storedToken.FamilyID, "REUSE_DETECTED")
		return nil, ErrTokenCompromised
	}

	// Expiry check
	if storedToken.ExpiresAt.Before(time.Now()) {
		_ = u.authRepo.RevokeToken(ctx, storedToken.ID, "EXPIRED")
		return nil, ErrTokenExpired
	}

	// Generate successor refresh token
	newRawToken, newTokenHash, err := GenerateCSPRNGRefreshToken()
	if err != nil {
		return nil, err
	}

	successorToken := &domain.RefreshToken{
		UserID:    storedToken.UserID,
		TokenHash: newTokenHash,
		FamilyID:  storedToken.FamilyID,
		ExpiresAt: time.Now().Add(u.refreshTTL),
		UserAgent: &userAgent,
		IPAddress: &ipAddress,
	}

	if err := u.authRepo.RotateRefreshToken(ctx, storedToken.ID, successorToken); err != nil {
		if errors.Is(err, repository.ErrTokenAlreadyRevoked) {
			_ = u.authRepo.RevokeFamily(ctx, storedToken.FamilyID, "REUSE_DETECTED")
			return nil, ErrTokenCompromised
		}
		return nil, fmt.Errorf("failed to rotate refresh token: %w", err)
	}

	// Fetch user details for new access token
	user, err := u.userRepo.GetByID(ctx, storedToken.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	caps, err := u.userRepo.GetUserCapabilities(ctx, storedToken.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get capabilities: %w", err)
	}

	emailVal := ""
	if user.EmailNormalized != nil {
		emailVal = *user.EmailNormalized
	}

	accessTok, err := security.GenerateAccessToken(u.jwtSecret, user.ID.String(), emailVal, caps, u.accessTTL)
	if err != nil {
		return nil, fmt.Errorf("failed to mint access token: %w", err)
	}

	return &domain.RefreshTokenResponse{
		AccessToken:  accessTok,
		RefreshToken: newRawToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(u.accessTTL.Seconds()),
	}, nil
}

func (u *authUsecase) Logout(ctx context.Context, req domain.LogoutRequest) error {
	rawToken := strings.TrimSpace(req.RefreshToken)
	if rawToken == "" {
		return nil
	}

	tokenDigest := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(tokenDigest[:])

	storedToken, err := u.authRepo.GetRefreshTokenByHash(ctx, tokenHash)
	if err == nil && !storedToken.IsRevoked {
		_ = u.authRepo.RevokeToken(ctx, storedToken.ID, "LOGOUT")
		u.recordAudit(ctx, audit.Entry{
			ActorUserID: &storedToken.UserID,
			ActionCode:  "AUTH_SESSION_REVOKED",
			TargetType:  "auth_refresh_token",
			TargetID:    &storedToken.ID,
			ReasonCode:  ptrString("LOGOUT"),
		})
	}

	return nil
}

// ptrString returns a pointer to the given string literal.
func ptrString(s string) *string { return &s }

func (u *authUsecase) SendOTP(ctx context.Context, req domain.SendOTPRequest) (*domain.SendOTPResponse, error) {
	phone := strings.TrimSpace(req.PhoneE164)
	email := strings.TrimSpace(req.Email)
	purpose := strings.TrimSpace(req.Purpose)

	if phone == "" && email == "" {
		return nil, fmt.Errorf("%w: phone_e164 or email is required", ErrInvalidInput)
	}
	if purpose == "" {
		purpose = "PHONE_VERIFICATION"
	}

	target := phone
	if target == "" {
		target = strings.ToLower(email)
	}

	// Rate Limiting: 1 send / 60s cooldown, max 3 sends / 10m burst window
	if u.limiter != nil {
		res, err := u.limiter.AllowOTPSend(ctx, target)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate OTP rate limit: %w", err)
		}
		if !res.Allowed {
			return nil, ErrTooManyRequests
		}
	}

	// Generate 6-digit keyed OTP (Condition F-01: Keyed HMAC-SHA256 digest)
	code, codeHash, err := security.GenerateKeyedOTP(u.otpSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to generate keyed OTP: %w", err)
	}

	var phonePtr, emailPtr *string
	if phone != "" {
		phonePtr = &phone
	}
	if email != "" {
		normEmail := strings.ToLower(email)
		emailPtr = &normEmail
	}

	challenge := &domain.OTPChallenge{
		PhoneE164:       phonePtr,
		EmailNormalized: emailPtr,
		Purpose:         purpose,
		CodeHash:        codeHash,
		MaxAttempts:     3,
		Status:          "PENDING",
		ExpiresAt:       time.Now().Add(3 * time.Minute),
	}

	if err := u.authRepo.CreateOTPChallenge(ctx, challenge); err != nil {
		return nil, fmt.Errorf("failed to persist OTP challenge: %w", err)
	}

	deliveryChannel := "WHATSAPP"
	if phone == "" && email != "" {
		deliveryChannel = "EMAIL"
	}

	if u.notifier != nil {
		_ = u.notifier.NotifyOTP(ctx, target, code, purpose, deliveryChannel)
	}

	return &domain.SendOTPResponse{
		ChallengeID:      challenge.ID.String(),
		ExpiresInSeconds: 180,
		DeliveryChannel:  deliveryChannel,
	}, nil
}

func (u *authUsecase) VerifyOTP(ctx context.Context, req domain.VerifyOTPRequest) (*domain.VerifyOTPResponse, error) {
	challengeIDStr := strings.TrimSpace(req.ChallengeID)
	otpCode := strings.TrimSpace(req.OTPCode)

	if challengeIDStr == "" || !security.ValidateOTPFormat(otpCode) {
		return nil, fmt.Errorf("%w: invalid challenge_id or otp_code", ErrInvalidInput)
	}

	challengeID, err := uuid.Parse(challengeIDStr)
	if err != nil {
		return nil, fmt.Errorf("%w: challenge_id must be a valid UUID", ErrInvalidInput)
	}

	challenge, err := u.authRepo.GetOTPChallengeByID(ctx, challengeID)
	if err != nil {
		if errors.Is(err, repository.ErrChallengeNotFound) {
			return nil, ErrInvalidOTP
		}
		return nil, fmt.Errorf("failed to get challenge: %w", err)
	}

	if challenge.Status != "PENDING" {
		return nil, ErrInvalidOTP
	}

	if challenge.ExpiresAt.Before(time.Now()) {
		_ = u.authRepo.MarkOTPStatus(ctx, challengeID, "EXPIRED")
		return nil, ErrOTPExpired
	}

	if challenge.AttemptCount >= challenge.MaxAttempts {
		_ = u.authRepo.MarkOTPStatus(ctx, challengeID, "FAILED")
		return nil, ErrMaxAttemptsExceeded
	}

	// Verify using constant-time Keyed HMAC-SHA256
	match := security.VerifyKeyedOTP(otpCode, challenge.CodeHash, u.otpSecret)
	if !match {
		newAttempts, _ := u.authRepo.IncrementOTPAttempts(ctx, challengeID)
		if newAttempts >= challenge.MaxAttempts {
			_ = u.authRepo.MarkOTPStatus(ctx, challengeID, "FAILED")
			return nil, ErrMaxAttemptsExceeded
		}
		return nil, ErrIncorrectOTP
	}

	// Mark verified
	if err := u.authRepo.MarkOTPVerified(ctx, challengeID); err != nil {
		return nil, fmt.Errorf("failed to mark OTP verified: %w", err)
	}

	// Generate verification token
	mac := hmac.New(sha256.New, u.otpSecret)
	mac.Write([]byte(challenge.ID.String() + ":" + time.Now().UTC().Format(time.RFC3339)))
	vtok := "vtok_" + hex.EncodeToString(mac.Sum(nil))[:32]

	return &domain.VerifyOTPResponse{
		Verified:          true,
		VerificationToken: vtok,
	}, nil
}

func (u *authUsecase) GetCurrentUser(ctx context.Context, userID uuid.UUID) (*domain.CurrentUserResponse, error) {
	user, err := u.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch user: %w", err)
	}

	profile, err := u.userRepo.GetUserProfile(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch user profile: %w", err)
	}

	caps, err := u.userRepo.GetUserCapabilities(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch user capabilities: %w", err)
	}

	memberships, err := u.userRepo.GetUserHostMemberships(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch host memberships: %w", err)
	}

	resp := &domain.CurrentUserResponse{
		ID:              user.ID.String(),
		DisplayName:     profile.DisplayName,
		Capabilities:    caps,
		HostMemberships: memberships,
	}
	if user.EmailNormalized != nil {
		resp.EmailNormalized = *user.EmailNormalized
	}
	if user.PhoneE164 != nil {
		resp.PhoneE164 = *user.PhoneE164
	}

	return resp, nil
}
