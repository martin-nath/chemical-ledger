package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/martin-nath/chemical-ledger/db"
	"github.com/martin-nath/chemical-ledger/handlers"
	"github.com/martin-nath/chemical-ledger/migrate"
)

func main() {
	// Parse command-line flags
	resetFlag := flag.Bool("reset", false, "Drop all tables before running migrations")
	flag.Parse()

	// Initialize the database
	db.InitDB("./chemical_ledger.db")
	defer db.Db.Close()

	// Drop tables if the reset flag is set
	if *resetFlag {
		if err := migrate.DropTables(db.Db); err != nil {
			log.Fatalf("Failed to drop tables: %v", err)
		}
		fmt.Println("Tables dropped successfully!")
	}

	// Create tables if they don't exist
	if err := migrate.CreateTables(db.Db); err != nil {
		log.Fatalf("Failed to create tables: %v", err)
	}
	fmt.Println("Tables created successfully!")

	// Set up routes
	http.HandleFunc("/insert", handlers.InsertData) // POST /transaction
	http.HandleFunc("/fetch", handlers.GetData)     // GET /transactions
	http.HandleFunc("/update", handlers.UpdateEntryHandler)
	// http.HandleFunc("/delete", handlers.DeleteEntryHandler)

	// Start the server
	fmt.Println("Server is running on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
