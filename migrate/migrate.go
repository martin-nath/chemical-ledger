package migrate

import (
    "database/sql"
    "fmt"
)

// Migrate creates the required tables if they do not exist
func Migrate(db *sql.DB) error {
    // Create the `chemicals` table with net_stock field (default 0)
    chemicalsTable := `
    CREATE TABLE IF NOT EXISTS chemicals (
        id TEXT PRIMARY KEY,      -- custom chemical id (camelCase name)
        name TEXT NOT NULL UNIQUE,
        net_stock INTEGER NOT NULL DEFAULT 0
    );
`
    if _, err := db.Exec(chemicalsTable); err != nil {
        return fmt.Errorf("failed to create chemicals table: %w", err)
    }

    // Create the `transactions` table
    transactionsTable := `
    CREATE TABLE IF NOT EXISTS transactions (
        id TEXT PRIMARY KEY,      -- custom transaction id (I/O + camelCase chemical name + timestamp)
        type TEXT NOT NULL CHECK (type IN ('Incoming','Outgoing')),
        date TEXT NOT NULL,
        chemical_id TEXT NOT NULL,
        no_of_units INTEGER NOT NULL,
        quantity_per_unit INTEGER NOT NULL,
        unit TEXT NOT NULL,
        remark TEXT,
        voucher_no TEXT,
        FOREIGN KEY (chemical_id) REFERENCES chemicals(id)
    );
`
    if _, err := db.Exec(transactionsTable); err != nil {
        return fmt.Errorf("failed to create transactions table: %w", err)
    }

    return nil
}

// DropTables drops the `chemicals` and `transactions` tables
func DropTables(db *sql.DB) error {
    if _, err := db.Exec("DROP TABLE IF EXISTS transactions;"); err != nil {
        return fmt.Errorf("failed to drop transactions table: %w", err)
    }
    if _, err := db.Exec("DROP TABLE IF EXISTS chemicals;"); err != nil {
        return fmt.Errorf("failed to drop chemicals table: %w", err)
    }
    return nil
}