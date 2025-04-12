package db

import (
    "database/sql"
    "fmt"
    "log"

    _ "github.com/mattn/go-sqlite3"
)

// Chemical represents a chemical with its net stock.
type Chemical struct {
    ID       string `json:"id"`       // This will be the chemical name in CamelCase.
    Name     string `json:"name"`
    NetStock int    `json:"net_stock"`  // Current stock (incoming adds, outgoing subtracts)
}

// Transaction represents a chemical transaction.
type Transaction struct {
    ID              string `json:"id"`              // Custom transaction ID (I/O + CamelCase chemical name + timestamp)
    Type            string `json:"type"`            // "Incoming" or "Outgoing"
    Date            string `json:"date"`
    CompoundName    string `json:"compound_name"`   // Original chemical name (for display)
    NoOfUnits       int    `json:"no_of_units"`     // If not provided, default value 1 will be used.
    QuantityPerUnit int    `json:"quantity_per_unit"`
    Unit            string `json:"unit"`
    Remark          string `json:"remark"`
    VoucherNo       string `json:"voucher_no"`
}

var Db *sql.DB

func InitDB(dataSourceName string) {
    var err error
    Db, err = sql.Open("sqlite3", dataSourceName)
    if err != nil {
        log.Fatalf("Failed to open database: %v", err)
    }
    if err = Db.Ping(); err != nil {
        log.Fatalf("Failed to connect to database: %v", err)
    }
    fmt.Println("Connected to the database")
}