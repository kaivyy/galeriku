package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
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

func SetupRoutes() *http.ServeMux {
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		
		reqID := r.Header.Get("X-Request-ID")
		if reqID == "" {
			reqID = "req_" + uuid.New().String()[:12]
		}

		resp := SuccessEnvelope{
			Data: map[string]interface{}{
				"status":    "UP",
				"service":   "galeriku-core-api",
				"version":   "0.1.0-draft",
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
