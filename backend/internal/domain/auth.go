package domain

import (
	"time"

	"github.com/google/uuid"
)

// UserProfile represents the public user profile in user_profiles.
type UserProfile struct {
	ID             uuid.UUID `json:"id"`
	UserID         uuid.UUID `json:"user_id"`
	DisplayName    string    `json:"display_name"`
	AvatarAssetKey *string   `json:"avatar_asset_key,omitempty"`
	Locale         string    `json:"locale"`
	CreatedAt      time.Time `json:"created_at"`
}

// UserCredential holds password authentication state in user_credentials.
type UserCredential struct {
	ID             uuid.UUID  `json:"id"`
	UserID         uuid.UUID  `json:"user_id"`
	PasswordHash   string     `json:"-"`
	PasswordAlgo   string     `json:"password_algo"`
	FailedAttempts int        `json:"failed_attempts"`
	LockedUntil    *time.Time `json:"locked_until,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// RefreshToken represents a stateful session token in auth_refresh_tokens.
type RefreshToken struct {
	ID            uuid.UUID `json:"id"`
	UserID        uuid.UUID `json:"user_id"`
	TokenHash     string    `json:"token_hash"`
	FamilyID      uuid.UUID `json:"family_id"`
	IsRevoked     bool      `json:"is_revoked"`
	RevokedReason *string   `json:"revoked_reason,omitempty"`
	ExpiresAt     time.Time `json:"expires_at"`
	CreatedAt     time.Time `json:"created_at"`
	UserAgent     *string   `json:"user_agent,omitempty"`
	IPAddress     *string   `json:"ip_address,omitempty"`
}

// OTPChallenge represents a verification challenge in auth_otp_challenges.
type OTPChallenge struct {
	ID              uuid.UUID  `json:"id"`
	PhoneE164       *string    `json:"phone_e164,omitempty"`
	EmailNormalized *string    `json:"email_normalized,omitempty"`
	Purpose         string     `json:"purpose"`
	CodeHash        string     `json:"-"`
	AttemptCount    int        `json:"attempt_count"`
	MaxAttempts     int        `json:"max_attempts"`
	Status          string     `json:"status"`
	ExpiresAt       time.Time  `json:"expires_at"`
	VerifiedAt      *time.Time `json:"verified_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

// --- DTOs (Data Transfer Objects) ---

// RegisterRequest payload for POST /api/v1/auth/register.
type RegisterRequest struct {
	Email       string `json:"email"`
	PhoneE164   string `json:"phone_e164"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

// RegisterResponse payload returned upon successful account registration.
type RegisterResponse struct {
	ID              string   `json:"id"`
	EmailNormalized string   `json:"email_normalized,omitempty"`
	PhoneE164       string   `json:"phone_e164,omitempty"`
	DisplayName     string   `json:"display_name"`
	Capabilities    []string `json:"capabilities"`
}

// LoginRequest payload for POST /api/v1/auth/login.
type LoginRequest struct {
	Identifier string `json:"identifier"`
	Password   string `json:"password"`
}

// UserSummary nested payload in LoginResponse.
type UserSummary struct {
	ID           string   `json:"id"`
	DisplayName  string   `json:"display_name"`
	Capabilities []string `json:"capabilities"`
}

// LoginResponse payload for POST /api/v1/auth/login.
type LoginResponse struct {
	AccessToken  string      `json:"access_token"`
	RefreshToken string      `json:"refresh_token"`
	TokenType    string      `json:"token_type"`
	ExpiresIn    int64       `json:"expires_in"`
	User         UserSummary `json:"user"`
}

// RefreshTokenRequest payload for POST /api/v1/auth/refresh.
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// RefreshTokenResponse payload for POST /api/v1/auth/refresh.
type RefreshTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}

// LogoutRequest payload for POST /api/v1/auth/logout.
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// SendOTPRequest payload for POST /api/v1/auth/otp/send.
type SendOTPRequest struct {
	PhoneE164 string `json:"phone_e164,omitempty"`
	Email     string `json:"email,omitempty"`
	Purpose   string `json:"purpose"`
}

// SendOTPResponse payload for POST /api/v1/auth/otp/send.
type SendOTPResponse struct {
	ChallengeID      string `json:"challenge_id"`
	ExpiresInSeconds int64  `json:"expires_in_seconds"`
	DeliveryChannel  string `json:"delivery_channel"`
}

// VerifyOTPRequest payload for POST /api/v1/auth/otp/verify.
type VerifyOTPRequest struct {
	ChallengeID string `json:"challenge_id"`
	OTPCode     string `json:"otp_code"`
}

// VerifyOTPResponse payload for POST /api/v1/auth/otp/verify.
type VerifyOTPResponse struct {
	Verified          bool   `json:"verified"`
	VerificationToken string `json:"verification_token"`
}

// HostMembershipSummary summary of organization membership for auth/me.
type HostMembershipSummary struct {
	OrganizationID   string `json:"organization_id"`
	OrganizationName string `json:"organization_name"`
	Role             string `json:"role"`
}

// CurrentUserResponse payload for GET /api/v1/auth/me.
type CurrentUserResponse struct {
	ID              string                  `json:"id"`
	DisplayName     string                  `json:"display_name"`
	EmailNormalized string                  `json:"email_normalized,omitempty"`
	PhoneE164       string                  `json:"phone_e164,omitempty"`
	Capabilities    []string                `json:"capabilities"`
	HostMemberships []HostMembershipSummary `json:"host_memberships"`
}
