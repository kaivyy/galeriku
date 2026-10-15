package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/galeriku/core/internal/config"
	"github.com/galeriku/core/internal/domain"
	"github.com/galeriku/core/internal/handler"
	"github.com/galeriku/core/internal/repository"
	"github.com/galeriku/core/internal/security"
	"github.com/galeriku/core/internal/usecase"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func getTestEnv(t *testing.T) (*pgxpool.Pool, *redis.Client, http.Handler, func()) {
	if testing.Short() {
		t.Skip("skipping live handler integration test in short mode")
	}

	cfg := config.LoadConfig()
	dbHost := os.Getenv("DB_HOST")
	if dbHost == "" {
		dbHost = "localhost"
	}
	dbPort := os.Getenv("DB_PORT")
	if dbPort == "" {
		dbPort = "5433"
	}
	connStr := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		cfg.DBUser, cfg.DBPassword, dbHost, dbPort, cfg.DBName)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Skipf("cannot connect to postgres: %v", err)
	}

	redisHost := os.Getenv("REDIS_HOST")
	if redisHost == "" {
		redisHost = "localhost"
	}
	redisPort := os.Getenv("REDIS_PORT")
	if redisPort == "" {
		redisPort = "6380"
	}
	rdb := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%s", redisHost, redisPort),
	})

	userRepo := repository.NewPostgresUserRepository(pool)
	authRepo := repository.NewPostgresAuthRepository(pool)
	limiter := security.NewRedisRateLimiter(rdb)

	jwtSecret := "test_super_secret_jwt_key_32_bytes_long_12345"
	otpSecret := "test_otp_secret_key_32_bytes_long_67890"

	authUsecase := usecase.NewAuthUsecase(
		userRepo,
		authRepo,
		limiter,
		jwtSecret,
		otpSecret,
		15*time.Minute,
		30*24*time.Hour,
		nil,
	)

	authHandler := handler.NewAuthHandler(authUsecase, jwtSecret)
	appRouter := handler.SetupRoutesWithAuth(pool, authHandler)

	cleanup := func() {
		pool.Close()
		_ = rdb.Close()
	}

	return pool, rdb, appRouter, cleanup
}

func TestAuthHandler_Register_SuccessAndDuplicate(t *testing.T) {
	_, _, router, cleanup := getTestEnv(t)
	defer cleanup()

	uniqueID := uuid.New().String()[:8]
	email := fmt.Sprintf("reg_%s@galeriku.com", uniqueID)
	phone := fmt.Sprintf("+62812%08d", time.Now().UnixNano()%100000000)

	reqBody := domain.RegisterRequest{
		Email:       email,
		PhoneE164:   phone,
		Password:    "SecurePass123!",
		DisplayName: "Budi Santoso",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	// 1. Success Registration
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp handler.SuccessEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response envelope: %v", err)
	}
	if !resp.Success {
		t.Error("expected response success: true")
	}

	dataMap, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected data to be map[string]interface{}, got: %T", resp.Data)
	}
	if dataMap["display_name"] != "Budi Santoso" {
		t.Errorf("expected display_name 'Budi Santoso', got: %v", dataMap["display_name"])
	}
	caps, _ := dataMap["capabilities"].([]interface{})
	if len(caps) != 1 || caps[0] != "BUYER" {
		t.Errorf("expected capabilities [BUYER], got: %v", caps)
	}

	// 2. Duplicate Registration -> 409 Conflict
	reqDup := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(bodyBytes))
	reqDup.Header.Set("Content-Type", "application/json")
	recDup := httptest.NewRecorder()

	router.ServeHTTP(recDup, reqDup)

	if recDup.Code != http.StatusConflict {
		t.Fatalf("expected status 409 for duplicate registration, got %d", recDup.Code)
	}
}

func TestAuthHandler_Login_SuccessAndFailure(t *testing.T) {
	_, _, router, cleanup := getTestEnv(t)
	defer cleanup()

	uniqueID := uuid.New().String()[:8]
	email := fmt.Sprintf("login_%s@galeriku.com", uniqueID)
	phone := fmt.Sprintf("+62814%08d", time.Now().UnixNano()%100000000)
	password := "LoginSecret123!"

	// Register account first
	regBody, _ := json.Marshal(domain.RegisterRequest{
		Email:       email,
		PhoneE164:   phone,
		Password:    password,
		DisplayName: "Login Tester",
	})
	reqReg := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(regBody))
	reqReg.Header.Set("Content-Type", "application/json")
	recReg := httptest.NewRecorder()
	router.ServeHTTP(recReg, reqReg)
	if recReg.Code != http.StatusCreated {
		t.Fatalf("registration prerequisite failed: %s", recReg.Body.String())
	}

	// 1. Successful Login
	loginBody, _ := json.Marshal(domain.LoginRequest{
		Identifier: email,
		Password:   password,
	})
	reqLogin := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(loginBody))
	reqLogin.Header.Set("Content-Type", "application/json")
	recLogin := httptest.NewRecorder()

	router.ServeHTTP(recLogin, reqLogin)

	if recLogin.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body: %s)", recLogin.Code, recLogin.Body.String())
	}

	var resp handler.SuccessEnvelope
	json.Unmarshal(recLogin.Body.Bytes(), &resp)
	dataMap := resp.Data.(map[string]interface{})

	accessToken, ok1 := dataMap["access_token"].(string)
	refreshToken, ok2 := dataMap["refresh_token"].(string)
	if !ok1 || accessToken == "" {
		t.Error("expected non-empty access_token")
	}
	if !ok2 || refreshToken == "" {
		t.Error("expected non-empty refresh_token")
	}
	if dataMap["token_type"] != "Bearer" {
		t.Errorf("expected token_type 'Bearer', got: %v", dataMap["token_type"])
	}

	// 2. Failed Login (Wrong Password)
	wrongLoginBody, _ := json.Marshal(domain.LoginRequest{
		Identifier: email,
		Password:   "WrongPass123!",
	})
	reqWrong := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(wrongLoginBody))
	reqWrong.Header.Set("Content-Type", "application/json")
	recWrong := httptest.NewRecorder()

	router.ServeHTTP(recWrong, reqWrong)

	if recWrong.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 for wrong password, got %d", recWrong.Code)
	}

	// 3. Failed Login (Non-existent user)
	nonExistentBody, _ := json.Marshal(domain.LoginRequest{
		Identifier: "nobody@galeriku.com",
		Password:   "SomePass123!",
	})
	reqNobody := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(nonExistentBody))
	reqNobody.Header.Set("Content-Type", "application/json")
	recNobody := httptest.NewRecorder()

	router.ServeHTTP(recNobody, reqNobody)

	if recNobody.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 for non-existent user, got %d", recNobody.Code)
	}
}

func TestAuthHandler_RefreshAndReuseDetection(t *testing.T) {
	_, _, router, cleanup := getTestEnv(t)
	defer cleanup()

	uniqueID := uuid.New().String()[:8]
	email := fmt.Sprintf("refresh_%s@galeriku.com", uniqueID)
	password := "RefreshSecret123!"

	// Register and login
	regBody, _ := json.Marshal(domain.RegisterRequest{
		Email:       email,
		Password:    password,
		DisplayName: "RTR Tester",
	})
	reqReg := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(regBody))
	reqReg.Header.Set("Content-Type", "application/json")
	recReg := httptest.NewRecorder()
	router.ServeHTTP(recReg, reqReg)

	loginBody, _ := json.Marshal(domain.LoginRequest{
		Identifier: email,
		Password:   password,
	})
	reqLogin := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(loginBody))
	reqLogin.Header.Set("Content-Type", "application/json")
	recLogin := httptest.NewRecorder()
	router.ServeHTTP(recLogin, reqLogin)

	var loginResp handler.SuccessEnvelope
	json.Unmarshal(recLogin.Body.Bytes(), &loginResp)
	loginData := loginResp.Data.(map[string]interface{})
	token1 := loginData["refresh_token"].(string)

	// 1. First refresh with token1 -> Success (Returns token2)
	refBody1, _ := json.Marshal(domain.RefreshTokenRequest{RefreshToken: token1})
	reqRef1 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(refBody1))
	reqRef1.Header.Set("Content-Type", "application/json")
	recRef1 := httptest.NewRecorder()

	router.ServeHTTP(recRef1, reqRef1)

	if recRef1.Code != http.StatusOK {
		t.Fatalf("expected status 200 on first refresh, got %d (body: %s)", recRef1.Code, recRef1.Body.String())
	}

	var ref1Resp handler.SuccessEnvelope
	json.Unmarshal(recRef1.Body.Bytes(), &ref1Resp)
	ref1Data := ref1Resp.Data.(map[string]interface{})
	token2 := ref1Data["refresh_token"].(string)

	if token2 == "" || token2 == token1 {
		t.Errorf("expected rotated token2 to be different from token1, got: %s", token2)
	}

	// 2. Reuse attack: Re-using token1 must trigger reuse detection and revoke family
	reqReuse := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(refBody1))
	reqReuse.Header.Set("Content-Type", "application/json")
	recReuse := httptest.NewRecorder()

	router.ServeHTTP(recReuse, reqReuse)

	if recReuse.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 on token reuse attack, got %d", recReuse.Code)
	}

	// 3. Now token2 (successor) must also be blocked because the entire family was terminated
	refBody2, _ := json.Marshal(domain.RefreshTokenRequest{RefreshToken: token2})
	reqRef2 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(refBody2))
	reqRef2.Header.Set("Content-Type", "application/json")
	recRef2 := httptest.NewRecorder()

	router.ServeHTTP(recRef2, reqRef2)

	if recRef2.Code != http.StatusUnauthorized {
		t.Fatalf("expected successor token to be revoked following reuse attack, got %d", recRef2.Code)
	}
}

func TestAuthHandler_OTPFlowAndAuthMe(t *testing.T) {
	_, _, router, cleanup := getTestEnv(t)
	defer cleanup()

	uniqueID := uuid.New().String()[:8]
	email := fmt.Sprintf("me_%s@galeriku.com", uniqueID)
	phone := fmt.Sprintf("+62817%08d", time.Now().UnixNano()%100000000)
	password := "MeSecret123!"

	// Register
	regBody, _ := json.Marshal(domain.RegisterRequest{
		Email:       email,
		PhoneE164:   phone,
		Password:    password,
		DisplayName: "Me Tester",
	})
	reqReg := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(regBody))
	reqReg.Header.Set("Content-Type", "application/json")
	recReg := httptest.NewRecorder()
	router.ServeHTTP(recReg, reqReg)

	// Login to get access token
	loginBody, _ := json.Marshal(domain.LoginRequest{
		Identifier: email,
		Password:   password,
	})
	reqLogin := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(loginBody))
	reqLogin.Header.Set("Content-Type", "application/json")
	recLogin := httptest.NewRecorder()
	router.ServeHTTP(recLogin, reqLogin)

	var loginResp handler.SuccessEnvelope
	json.Unmarshal(recLogin.Body.Bytes(), &loginResp)
	loginData := loginResp.Data.(map[string]interface{})
	accessToken := loginData["access_token"].(string)

	// 1. GET /api/v1/auth/me with valid Bearer token
	reqMe := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	reqMe.Header.Set("Authorization", "Bearer "+accessToken)
	recMe := httptest.NewRecorder()

	router.ServeHTTP(recMe, reqMe)

	if recMe.Code != http.StatusOK {
		t.Fatalf("expected status 200 for /auth/me, got %d (body: %s)", recMe.Code, recMe.Body.String())
	}

	var meResp handler.SuccessEnvelope
	json.Unmarshal(recMe.Body.Bytes(), &meResp)
	meData := meResp.Data.(map[string]interface{})
	if meData["display_name"] != "Me Tester" {
		t.Errorf("expected display_name 'Me Tester', got: %v", meData["display_name"])
	}
	if meData["email_normalized"] != email {
		t.Errorf("expected email %q, got: %v", email, meData["email_normalized"])
	}

	// 2. GET /api/v1/auth/me without Bearer token -> 401
	reqMeNoAuth := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	recMeNoAuth := httptest.NewRecorder()

	router.ServeHTTP(recMeNoAuth, reqMeNoAuth)

	if recMeNoAuth.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 without auth header, got %d", recMeNoAuth.Code)
	}

	// 3. OTP Send
	otpSendBody, _ := json.Marshal(domain.SendOTPRequest{
		PhoneE164: phone,
		Purpose:   "PHONE_VERIFICATION",
	})
	reqOTP := httptest.NewRequest(http.MethodPost, "/api/v1/auth/otp/send", bytes.NewReader(otpSendBody))
	reqOTP.Header.Set("Content-Type", "application/json")
	recOTP := httptest.NewRecorder()

	router.ServeHTTP(recOTP, reqOTP)

	if recOTP.Code != http.StatusOK {
		t.Fatalf("expected status 200 for OTP send, got %d (body: %s)", recOTP.Code, recOTP.Body.String())
	}

	var otpResp handler.SuccessEnvelope
	json.Unmarshal(recOTP.Body.Bytes(), &otpResp)
	otpData := otpResp.Data.(map[string]interface{})
	challengeID := otpData["challenge_id"].(string)
	if challengeID == "" {
		t.Fatal("expected non-empty challenge_id")
	}

	// 4. OTP Verify (Wrong Code -> 400)
	verifyBodyWrong, _ := json.Marshal(domain.VerifyOTPRequest{
		ChallengeID: challengeID,
		OTPCode:     "000000",
	})
	reqVerifyWrong := httptest.NewRequest(http.MethodPost, "/api/v1/auth/otp/verify", bytes.NewReader(verifyBodyWrong))
	reqVerifyWrong.Header.Set("Content-Type", "application/json")
	recVerifyWrong := httptest.NewRecorder()

	router.ServeHTTP(recVerifyWrong, reqVerifyWrong)

	if recVerifyWrong.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for wrong OTP code, got %d", recVerifyWrong.Code)
	}
}
