package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/galeriku/core/internal/domain"
	"github.com/galeriku/core/internal/middleware"
	"github.com/galeriku/core/internal/repository"
	"github.com/galeriku/core/internal/security"
	"github.com/galeriku/core/internal/usecase"
	"github.com/google/uuid"
)

type AuthHandler struct {
	usecase   usecase.AuthUsecase
	jwtSecret []byte
}

func NewAuthHandler(usecase usecase.AuthUsecase, jwtSecret string) *AuthHandler {
	return &AuthHandler{
		usecase:   usecase,
		jwtSecret: []byte(jwtSecret),
	}
}

func (h *AuthHandler) writeSuccess(w http.ResponseWriter, r *http.Request, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	reqID := middleware.GetRequestID(r.Context())
	resp := SuccessEnvelope{
		Success: true,
		Data:    data,
		Meta: SuccessMeta{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			RequestID: reqID,
		},
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *AuthHandler) writeError(w http.ResponseWriter, r *http.Request, statusCode int, code, message string, details interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if statusCode == http.StatusTooManyRequests {
		w.Header().Set("Retry-After", "60")
	}
	w.WriteHeader(statusCode)

	reqID := middleware.GetRequestID(r.Context())
	resp := ErrorEnvelope{
		Success: false,
		Error: ErrorBody{
			Code:      code,
			Message:   message,
			Details:   details,
			RequestID: reqID,
		},
		Meta: ErrorMeta{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			RequestID: reqID,
		},
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *AuthHandler) handleUsecaseError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, repository.ErrDuplicateAccount):
		h.writeError(w, r, http.StatusConflict, "ACCOUNT_CONFLICT", err.Error(), nil)
	case errors.Is(err, usecase.ErrInvalidInput):
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", err.Error(), nil)
	case errors.Is(err, security.ErrPasswordComplexity):
		h.writeError(w, r, http.StatusBadRequest, "INVALID_PASSWORD_COMPLEXITY", err.Error(), nil)
	case errors.Is(err, usecase.ErrInvalidCredentials):
		h.writeError(w, r, http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid identifier or password", nil)
	case errors.Is(err, usecase.ErrAccountInactive):
		h.writeError(w, r, http.StatusUnauthorized, "ACCOUNT_INACTIVE", "account is suspended or inactive", nil)
	case errors.Is(err, usecase.ErrTooManyRequests):
		h.writeError(w, r, http.StatusTooManyRequests, "TOO_MANY_REQUESTS", "rate limit exceeded, please retry later", nil)
	case errors.Is(err, usecase.ErrInvalidToken):
		h.writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or malformed refresh token", nil)
	case errors.Is(err, usecase.ErrTokenExpired):
		h.writeError(w, r, http.StatusUnauthorized, "TOKEN_EXPIRED", "refresh token has expired", nil)
	case errors.Is(err, usecase.ErrTokenCompromised):
		h.writeError(w, r, http.StatusUnauthorized, "TOKEN_REUSE_DETECTED", "token reuse detected, session terminated", nil)
	case errors.Is(err, usecase.ErrInvalidOTP),
		errors.Is(err, usecase.ErrOTPExpired),
		errors.Is(err, usecase.ErrMaxAttemptsExceeded),
		errors.Is(err, usecase.ErrIncorrectOTP):
		h.writeError(w, r, http.StatusBadRequest, "OTP_VERIFICATION_FAILED", err.Error(), nil)
	default:
		h.writeError(w, r, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "an unexpected error occurred", nil)
	}
}

// Register handles POST /api/v1/auth/register.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req domain.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "malformed JSON payload", nil)
		return
	}

	resp, err := h.usecase.Register(r.Context(), req)
	if err != nil {
		h.handleUsecaseError(w, r, err)
		return
	}

	h.writeSuccess(w, r, http.StatusCreated, resp)
}

// Login handles POST /api/v1/auth/login.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req domain.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "malformed JSON payload", nil)
		return
	}

	userAgent := r.UserAgent()
	ipAddress := r.RemoteAddr

	resp, err := h.usecase.Login(r.Context(), req, userAgent, ipAddress)
	if err != nil {
		h.handleUsecaseError(w, r, err)
		return
	}

	h.writeSuccess(w, r, http.StatusOK, resp)
}

// Refresh handles POST /api/v1/auth/refresh.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req domain.RefreshTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "malformed JSON payload", nil)
		return
	}

	userAgent := r.UserAgent()
	ipAddress := r.RemoteAddr

	resp, err := h.usecase.RefreshToken(r.Context(), req, userAgent, ipAddress)
	if err != nil {
		h.handleUsecaseError(w, r, err)
		return
	}

	h.writeSuccess(w, r, http.StatusOK, resp)
}

// Logout handles POST /api/v1/auth/logout.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var req domain.LogoutRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	_ = h.usecase.Logout(r.Context(), req)
	h.writeSuccess(w, r, http.StatusOK, map[string]bool{"logged_out": true})
}

// SendOTP handles POST /api/v1/auth/otp/send.
func (h *AuthHandler) SendOTP(w http.ResponseWriter, r *http.Request) {
	var req domain.SendOTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "malformed JSON payload", nil)
		return
	}

	resp, err := h.usecase.SendOTP(r.Context(), req)
	if err != nil {
		h.handleUsecaseError(w, r, err)
		return
	}

	h.writeSuccess(w, r, http.StatusOK, resp)
}

// VerifyOTP handles POST /api/v1/auth/otp/verify.
func (h *AuthHandler) VerifyOTP(w http.ResponseWriter, r *http.Request) {
	var req domain.VerifyOTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, r, http.StatusBadRequest, "INVALID_INPUT", "malformed JSON payload", nil)
		return
	}

	resp, err := h.usecase.VerifyOTP(r.Context(), req)
	if err != nil {
		h.handleUsecaseError(w, r, err)
		return
	}

	h.writeSuccess(w, r, http.StatusOK, resp)
}

// Me handles GET /api/v1/auth/me.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	var userID uuid.UUID
	if identity, ok := middleware.GetAuthIdentity(r.Context()); ok && identity != nil {
		userID = identity.UserID
	} else {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			h.writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "missing or malformed authorization header", nil)
			return
		}

		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := security.VerifyAccessToken(h.jwtSecret, tokenStr)
		if err != nil {
			h.writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or expired access token", nil)
			return
		}

		var errParse error
		userID, errParse = uuid.Parse(claims.Subject)
		if errParse != nil {
			h.writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "invalid subject token claim", nil)
			return
		}
	}

	resp, err := h.usecase.GetCurrentUser(r.Context(), userID)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			h.writeError(w, r, http.StatusNotFound, "USER_NOT_FOUND", "user account not found", nil)
		} else {
			h.writeError(w, r, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "an unexpected error occurred", nil)
		}
		return
	}

	h.writeSuccess(w, r, http.StatusOK, resp)
}
