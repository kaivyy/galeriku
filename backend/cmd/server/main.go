package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/galeriku/core/internal/config"
	"github.com/galeriku/core/internal/database"
	"github.com/galeriku/core/internal/handler"
)

func main() {
	cfg := config.LoadConfig()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := database.NewPostgresPool(ctx, cfg)
	if err != nil {
		log.Printf("[GALERIKU-CORE] Warning: database pool init failed: %v", err)
	} else {
		defer pool.Close()
		log.Println("[GALERIKU-CORE] PostgreSQL connection pool initialized.")
	}

	router := handler.SetupRoutes(pool)

	addr := fmt.Sprintf(":%s", cfg.AppPort)
	log.Printf("[GALERIKU-CORE] Server running on %s (ENV: %s)", addr, cfg.AppEnv)

	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
