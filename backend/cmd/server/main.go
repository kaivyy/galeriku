package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/galeriku/core/internal/config"
	"github.com/galeriku/core/internal/handler"
)

func main() {
	cfg := config.LoadConfig()

	router := handler.SetupRoutes()

	addr := fmt.Sprintf(":%s", cfg.AppPort)
	log.Printf("[GALERIKU-CORE] Server running on %s (ENV: %s)", addr, cfg.AppEnv)

	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
