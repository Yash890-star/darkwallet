package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ecom-scraper/internal/api"
	"ecom-scraper/internal/scheduler"
	"ecom-scraper/internal/scraper"
	"ecom-scraper/internal/storage"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "tracker.db"
	}

	// 1. Initialize SQLite Database
	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize SQLite database: %v", err)
	}
	defer store.Close()
	log.Printf("📦 SQLite database initialized: %s", dbPath)

	// 2. Initialize HTTP client with realistic browser headers & cookies
	httpClient := scraper.NewHTTPClient(20 * time.Second)

	// 3. Initialize Extensible Registry
	registry := scraper.NewRegistry()

	// 4. Register Amazon India Scraper
	amazonScraper := scraper.NewAmazonScraper(httpClient)
	registry.Register(amazonScraper)

	// 5. Set Fallback Scraper for generic e-commerce & open-graph metadata
	registry.SetFallback(scraper.NewGenericScraper(httpClient))

	// 6. Initialize Background Cron Scheduler
	sched := scheduler.NewScheduler(store, registry)
	sched.Start()
	defer sched.Stop()

	// 7. Initialize API Handlers
	handler := api.NewHandler(registry, store, sched)

	// 8. Router Setup
	mux := http.NewServeMux()

	// Auth Routes
	mux.HandleFunc("/api/auth/register", handler.HandleRegister)
	mux.HandleFunc("/api/auth/login", handler.HandleLogin)
	mux.HandleFunc("/api/auth/logout", handler.HandleLogout)
	mux.HandleFunc("/api/auth/me", handler.HandleMe)

	// API Routes
	mux.HandleFunc("/health", handler.HealthCheck)
	mux.HandleFunc("/api/platforms", handler.GetPlatforms)
	mux.HandleFunc("/api/intervals", handler.GetIntervals)
	mux.HandleFunc("/api/scrape", handler.HandleScrape)
	mux.HandleFunc("/api/track", handler.HandleTrack)
	mux.HandleFunc("/api/track/untrack", handler.HandleUntrack)
	mux.HandleFunc("/api/tracked", handler.HandleListTracked)
	mux.HandleFunc("/api/tracked/", handler.HandleTrackedAction)
	mux.HandleFunc("/api/history", handler.HandleHistory)

	// Static frontend Web UI
	fs := http.FileServer(http.Dir("./web"))
	mux.Handle("/", fs)

	// Wrap with CORS
	serverHandler := handler.EnableCORS(mux)

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      serverHandler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown channel
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("====================================================")
		log.Printf("🛍️  E-Commerce Price Tracker Backend is running!")
		log.Printf("🌐 Local Web UI & API: http://localhost:%s", port)
		log.Printf("📡 API Endpoints:")
		log.Printf("    - POST/GET /api/scrape")
		log.Printf("    - POST     /api/track       (Cron Schedule)")
		log.Printf("    - GET      /api/tracked     (List Tracked Items)")
		log.Printf("    - GET      /api/history     (Price vs Time)")
		log.Printf("📋 Supported Platforms: %s", amazonScraper.PlatformName())
		log.Printf("====================================================")

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	<-stopChan
	log.Println("Shutting down server gracefully...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Server stopped cleanly.")
}
