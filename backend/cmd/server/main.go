package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/galeriku/core/internal/audit"
	"github.com/galeriku/core/internal/config"
	"github.com/galeriku/core/internal/database"
	"github.com/galeriku/core/internal/handler"
	"github.com/galeriku/core/internal/messaging"
	"github.com/galeriku/core/internal/repository"
	"github.com/galeriku/core/internal/security"
	"github.com/galeriku/core/internal/storage"
	"github.com/galeriku/core/internal/usecase"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg := config.LoadConfig()

	if cfg.AppEnv == "production" {
		if cfg.JWTSecret == "default_dev_secret_change_in_production" || cfg.OTPSecret == "default_otp_dev_secret_key_min_32_bytes_long!" {
			log.Fatal("[FATAL] Production startup rejected: default dev secrets detected for JWT_SECRET or OTP_SECRET_KEY")
		}
		if cfg.BankKey == "default_bank_cipher_key_32byte!!" {
			log.Fatal("[FATAL] Production startup rejected: default BANK_CIPHER_KEY detected; payout account numbers would be encrypted under a repository-visible key")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := database.NewPostgresPool(ctx, cfg)
	var authHandler *handler.AuthHandler
	var hostHandler *handler.HostApplicationHandler
	var membershipHandler *handler.HostMembershipHandler
	var bankHandler *handler.BankAccountHandler
	var invitationHandler *handler.InvitationHandler
	var eventTreeHandler *handler.EventTreeHandler
	var uploadHandler *handler.UploadBatchHandler
	var multipartHandler *handler.MultipartUploadHandler
	var finalizeHandler *handler.BatchFinalizeHandler
	var publisher messaging.Publisher = messaging.NoopPublisher{}
	var tenantRepo repository.TenantRepository

	if err != nil {
		log.Printf("[GALERIKU-CORE] Warning: database pool init failed: %v", err)
	} else {
		defer pool.Close()
		log.Println("[GALERIKU-CORE] PostgreSQL connection pool initialized.")

		rdb := redis.NewClient(&redis.Options{
			Addr: fmt.Sprintf("%s:%s", cfg.RedisHost, cfg.RedisPort),
		})
		defer rdb.Close()

		userRepo := repository.NewPostgresUserRepository(pool)
		authRepo := repository.NewPostgresAuthRepository(pool)
		limiter := security.NewRedisRateLimiter(rdb)
		auditSvc := audit.NewService(pool)

		authUsecase := usecase.NewAuthUsecase(
			userRepo,
			authRepo,
			limiter,
			cfg.JWTSecret,
			cfg.OTPSecret,
			15*time.Minute,
			30*24*time.Hour,
			auditSvc,
		)
		authHandler = handler.NewAuthHandler(authUsecase, cfg.JWTSecret)

		hostRepo := repository.NewPostgresHostRepository(pool)
		tenantRepo = repository.NewPostgresTenantRepository(pool)
		membershipHandler = handler.NewHostMembershipHandler(usecase.NewHostMembershipUsecase(hostRepo, auditSvc))

		invitationRepo := repository.NewPostgresInvitationRepository(pool)
		invitationHandler = handler.NewInvitationHandler(usecase.NewInvitationUsecase(invitationRepo, auditSvc))

		eventRepo := repository.NewPostgresEventRepository(pool)
		eventTreeHandler = handler.NewEventTreeHandler(usecase.NewEventTreeUsecase(eventRepo, auditSvc))

		uploadRepo := repository.NewPostgresUploadRepository(pool)
		uploadHandler = handler.NewUploadBatchHandler(usecase.NewUploadBatchUsecase(uploadRepo, auditSvc))

		if bankCipher, cipherErr := security.NewBankAccountCipher([]byte(cfg.BankKey)); cipherErr != nil {
			log.Printf("[GALERIKU-CORE] Warning: bank cipher init failed, payout account endpoints disabled: %v", cipherErr)
		} else {
			bankRepo := repository.NewPostgresBankAccountRepository(pool)
			bankHandler = handler.NewBankAccountHandler(usecase.NewBankAccountUsecase(bankRepo, bankCipher))
		}

		if s3svc, s3err := storage.NewS3Service(cfg.S3Endpoint, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3UseSSL); s3err != nil {
			log.Printf("[GALERIKU-CORE] Warning: S3 service init failed, Host document and upload endpoints disabled: %v", s3err)
		} else {
			hostUsecase := usecase.NewHostApplicationUsecase(hostRepo, s3svc)
			hostHandler = handler.NewHostApplicationHandler(hostUsecase)

			multipartRepo := repository.NewPostgresMultipartRepository(pool)
			multipartHandler = handler.NewMultipartUploadHandler(
				usecase.NewMultipartUploadUsecase(multipartRepo, uploadRepo, s3svc, auditSvc),
			)

			finalizeHandler = handler.NewBatchFinalizeHandler(
				usecase.NewBatchFinalizeUsecase(uploadRepo, s3svc, publisher, auditSvc),
			)
		}

		// The AI ingest publisher is optional: without a broker the phase 3 endpoints still
		// work, the pending ai_jobs rows simply wait for the Phase 4 worker to pick them up.
		if rabbitPub, pubErr := messaging.NewRabbitPublisher(
			fmt.Sprintf("amqp://%s:%s@%s:%s/", cfg.RabbitUser, cfg.RabbitPassword, cfg.RabbitHost, cfg.RabbitPort),
			cfg.RabbitPhotoQueue,
		); pubErr != nil {
			log.Printf("[GALERIKU-CORE] Warning: RabbitMQ publisher init failed, AI ingest events will not be dispatched: %v", pubErr)
		} else {
			publisher = rabbitPub
		}
	}

	router := handler.SetupRoutesWithFinalize(pool, authHandler, hostHandler, membershipHandler, tenantRepo, bankHandler, invitationHandler, eventTreeHandler, uploadHandler, multipartHandler, finalizeHandler, []byte(cfg.JWTSecret))

	addr := fmt.Sprintf(":%s", cfg.AppPort)
	srv := &http.Server{Addr: addr, Handler: router}

	// SIGINT and SIGTERM start a graceful drain: in-flight requests get 10 seconds to
	// finish, then the listener closes and the database pool is released before exit.
	ctxSignal, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("[GALERIKU-CORE] Server running on %s (ENV: %s)", addr, cfg.AppEnv)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	<-ctxSignal.Done()
	log.Println("[GALERIKU-CORE] Shutdown signal received, draining in-flight requests (10s budget).")

	drainCtx, cancelDrain := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelDrain()
	if err := srv.Shutdown(drainCtx); err != nil {
		log.Printf("[GALERIKU-CORE] Drain deadline exceeded: %v", err)
	}
	log.Println("[GALERIKU-CORE] Shutdown complete.")
}
