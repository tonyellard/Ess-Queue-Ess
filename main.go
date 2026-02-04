// SPDX-License-Identifier: Apache-2.0

package main

import (
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "9324" // Default SQS port for local development
	}

	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	// Routes
	r.Get("/health", healthHandler)
	r.HandleFunc("/*", rootHandler)

	log.Printf("Starting Ess-Queue-Ess on port %s", port)
	log.Printf("SQS endpoint: http://localhost:%s/", port)

	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
