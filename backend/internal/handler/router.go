package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SuccessEnvelope format standar respons sukses sesuai GALERIKU_API_ARCHITECTURE_SPEC.md Bab 4.1
type SuccessEnvelope struct {
	Data interface{} `json:"data"`
	Meta SuccessMeta `json:"meta"`
}

type SuccessMeta struct {
	RequestID  string  `json:"request_id"`
	NextCursor *string `json:"next_cursor"`
}

// ErrorEnvelope format standar respons galat sesuai GALERIKU_API_ARCHITECTURE_SPEC.md Bab 4.2
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

		overallStatus := "UP"
		httpCode := http.StatusOK

		dbStatus := "UNKNOWN"
		if pool != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 1*time.Second)
			defer cancel()
			if err := pool.Ping(ctx); err == nil {
				dbStatus = "UP"
			} else {
				dbStatus = "DOWN"
				overallStatus = "DEGRADED"
				httpCode = http.StatusServiceUnavailable
			}
		}

		resp := SuccessEnvelope{
			Data: map[string]interface{}{
				"status":    overallStatus,
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

		w.WriteHeader(httpCode)
		json.NewEncoder(w).Encode(resp)
	})

	return mux
}
