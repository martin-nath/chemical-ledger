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
    reset := flag.Bool("reset", false, "Drop all tables before running migrations")
    flag.Parse()

    // Initialize the database
    db.InitDB("./chemical_ledger.db")
    defer db.Db.Close()

    // Drop tables if the reset flag is set
    if *reset {
        if err := migrate.DropTables(db.Db); err != nil {
            log.Fatalf("Failed to drop tables: %v", err)
        }
        fmt.Println("Tables dropped successfully!")
    }

    // Run migrations
    if err := migrate.Migrate(db.Db); err != nil {
        log.Fatalf("Migration failed: %v", err)
    }

    fmt.Println("Migration completed successfully!")

    // Set up routes
    http.HandleFunc("/insert", handlers.InsertChemical) // POST /transaction
    http.HandleFunc("/fetch", handlers.FetchTransactions)  // GET /transactions

    // Start the server
    fmt.Println("Server is running on http://localhost:8080")
    log.Fatal(http.ListenAndServe(":8080", nil))
}