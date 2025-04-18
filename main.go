package main

import (
	"net/http"
	"os"

	"github.com/martin-nath/chemical-ledger/db"
	"github.com/martin-nath/chemical-ledger/handlers"
	"github.com/martin-nath/chemical-ledger/migrate"
	"github.com/sirupsen/logrus"
)

func main() {
	logFile, err := os.OpenFile("app.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		logrus.Fatal(err)
	}
	defer logFile.Close() // Ensure the file is closed when the application exits

	// Set the output of logrus to the file
	logrus.SetOutput(logFile)

	// Optionally, you can set the log format (e.g., JSON)
	logrus.SetFormatter(&logrus.JSONFormatter{})

	// Parse command-line flags

	// Initialize the database
	db.InitDB("./chemical_ledger.db")
	defer db.Db.Close()

	if err := migrate.DropTables(db.Db); err != nil {
		logrus.Fatalf("Failed to drop tables: %v", err)
	}
	logrus.Info("Tables dropped successfully!")

	if err := migrate.CreateTables(db.Db); err != nil {
		logrus.Fatalf("Failed to create tables: %v", err)
	}
	logrus.Info("Tables created successfully!")

	// Set up routes
	http.HandleFunc("/insert", handlers.InsertData) // POST /transaction
	http.HandleFunc("/fetch", handlers.GetData)     // GET /transactions
	http.HandleFunc("/update", handlers.UpdateData)

	// Start the server
	logrus.Info("Server is running on http://localhost:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		logrus.Fatalf("Server failed to start: %v", err)
	}
}
