package main

import (
	"chemical-ledger-backend/db"
	"chemical-ledger-backend/handlers"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	slogchi "github.com/samber/slog-chi"
)

//go:embed frontend/*
var frontendFiles embed.FS

func main() {
	// --- Logging and DB Setup ---
	if err := os.MkdirAll("./info", 0755); err != nil && !os.IsExist(err) {
		log.Fatal("failed to create './info' directory", "error", err)
	}
	logFile, err := os.OpenFile("./info/app.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatal("failed to open log file", "error", err)
	}
	defer logFile.Close()
	logger := slog.New(slog.NewJSONHandler(logFile, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := db.SetUpConnection("./info/chemical-ledger.db"); err != nil {
		slog.Error("failed to set up database connection", "err", err)
		panic(err)
	}
	if err := db.CreateTables(); err != nil {
		slog.Error("Failed to create tables", "err", err)
		panic(err)
	}

	// --- Use WaitGroup to manage goroutines ---
	var wg sync.WaitGroup
	wg.Add(2) // We are waiting for two servers to start

	// --- Start API and Frontend Servers Concurrently ---
	go startAPIServer(&wg)      // Run API on :8080
	go startFrontendServer(&wg) // Run Frontend on :3000

	// --- Open Browser and Wait ---
	frontendURL := "http://localhost:3000"
	slog.Info("Application starting...", "frontend_url", frontendURL)

	// Wait a moment for servers to initialize before opening the browser
	time.Sleep(1 * time.Second)
	openBrowser(frontendURL)

	// Block main from exiting until both goroutines are done
	wg.Wait()
}

// startAPIServer sets up and runs the backend API on port 8080.
func startAPIServer(wg *sync.WaitGroup) {
	defer wg.Done() // Signal that this goroutine is done when the function exits

	r := chi.NewRouter()
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{"http://localhost:3000"},
		AllowedMethods: []string{"GET", "POST", "PUT"},
	}))
	r.Use(slogchi.New(slog.Default()))
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			next.ServeHTTP(w, r)
		})
	})

	// API routes
	r.Post("/insert-compound", handlers.InsertCompoundHandler)
	r.Get("/get-compound", handlers.GetCompoundHandler)
	r.Put("/update-compound", handlers.UpdateCompoundHandler)
	r.Post("/insert-entry", handlers.InsertEntryHandler)
	r.Get("/get-entry", handlers.GetEntryHandler)
	r.Put("/update-entry", handlers.UpdateEntryHandler)

	slog.Info("Backend API server starting on :8080")
	if err := http.ListenAndServe(":8080", r); err != nil {
		slog.Error("Failed to start API server", "err", err)
		panic(err)
	}
}

// startFrontendServer serves the embedded frontend files on port 3000.
func startFrontendServer(wg *sync.WaitGroup) {
	defer wg.Done() // Signal that this goroutine is done when the function exits

	subFS, err := fs.Sub(frontendFiles, "frontend")
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(subFS)))

	slog.Info("Frontend server starting on :3000")
	if err := http.ListenAndServe(":3000", mux); err != nil {
		slog.Error("Failed to start frontend server", "err", err)
		panic(err)
	}
}

// openBrowser opens the given URL in the default browser on Windows.
func openBrowser(url string) {
	err := exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	if err != nil {
		slog.Warn("Failed to open browser automatically", "url", url, "err", err)
		fmt.Println("Please open the URL in your browser manually: " + url)
	} else {
		slog.Info("Default browser opened successfully")
		fmt.Println("Opening application in your default browser...")
		fmt.Println("If not opened, then open this url on your preferred browser: " + url)
	}
}
