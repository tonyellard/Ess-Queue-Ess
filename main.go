// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "", "Path to configuration file")
	flag.Parse()

	// Load configuration if provided
	if *configPath != "" {
		config, err := LoadConfig(*configPath)
		if err != nil {
			log.Printf("Warning: Failed to load config: %v", err)
		} else {
			log.Printf("Loaded configuration from %s", *configPath)
			if err := BootstrapQueues(config); err != nil {
				log.Fatalf("Failed to bootstrap queues: %v", err)
			}
			log.Printf("Bootstrapped %d queues from configuration", len(config.Queues))

			// Use port from config if not overridden by environment
			if os.Getenv("PORT") == "" && config.Server.Port > 0 {
				os.Setenv("PORT", strconv.Itoa(config.Server.Port))
			}
		}
	}

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
	r.Get("/admin", adminUIHandler)
	r.Get("/admin/api/queues", adminAPIHandler)
	r.HandleFunc("/*", rootHandler)

	log.Printf("Starting Ess-Queue-Ess on port %s", port)
	log.Printf("SQS endpoint: http://localhost:%s/", port)
	log.Printf("Admin UI: http://localhost:%s/admin", port)

	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
