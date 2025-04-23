package main

import (
	"net/http"
	"os"

	"github.com/martin-nath/chemical-ledger/db"
	"github.com/martin-nath/chemical-ledger/handlers"
	"github.com/martin-nath/chemical-ledger/migrate"
	"github.com/sirupsen/logrus"
)

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "http://localhost:3000")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Max-Age", "300")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func main() {
	logFile, err := os.OpenFile("app.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		logrus.SetOutput(os.Stderr)
		logrus.Fatalf("Failed to open log file: %v", err)
	}
	defer logFile.Close()

	logrus.SetOutput(logFile)
	logrus.SetFormatter(&logrus.JSONFormatter{})
	logrus.SetLevel(logrus.InfoLevel)

	db.InitDB("./chemical_ledger.db")
	defer db.Db.Close()


	if err := migrate.CreateTables(db.Db); err != nil {
		logrus.Fatalf("Failed to create tables: %v", err)
	}
	logrus.Info("Tables created successfully!")

	handlers.SetDatabase(db.Db)

	http.HandleFunc("/insert", handlers.InsertData)
	http.HandleFunc("/fetch", handlers.GetData)
	http.HandleFunc("/update", handlers.UpdateData)
	http.HandleFunc("/compound", handlers.CompoundName)

	handler := corsMiddleware(http.DefaultServeMux)

	port := "8080"
	logrus.Infof("Server is running on http://localhost:%s", port)
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		logrus.Fatalf("Server failed to start: %v", err)
	}
}
