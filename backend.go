package main

import (
	"chemical-ledger-backend/db"
	"chemical-ledger-backend/handlers"
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	slogchi "github.com/samber/slog-chi"
)

func main() {
	logFile, err := os.OpenFile("app.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		slog.Error("failed to open log file", "error", err)
		panic(err)
	}
	defer logFile.Close()

	logger := slog.New(slog.NewJSONHandler(logFile, &slog.HandlerOptions{}))
	slog.SetDefault(logger)

	if err = db.SetUpConnection("./chemical-ledger.db"); err != nil {
		slog.Error("failed to set up database connection")
		panic(err)
	}

	if err := db.CreateTables(); err != nil {
		slog.Error("Failed to create tables: " + err.Error())
		panic(err)
	}

	r := chi.NewRouter()

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:3000"},
		AllowedMethods:   []string{"GET", "POST", "PUT"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		AllowCredentials: false,
	}))

	r.Use(slogchi.New(logger))
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			next.ServeHTTP(w, r)
		})
	})

	r.Post("/insert-compound", handlers.InsertCompoundHandler)
	r.Get("/get-compound", handlers.GetCompoundHandler)
	r.Put("/update-compound", handlers.UpdateCompoundHandler)

	r.Post("/insert-entry", handlers.InsertEntryHandler)
	r.Get("/get-entry", handlers.GetEntryHandler)
	r.Put("/update-entry", handlers.UpdateEntryHandler)

	if err := http.ListenAndServe(":8080", r); err != nil {
		slog.Error("Failed to start server, err: " + err.Error())
		panic(err)
	}
}
