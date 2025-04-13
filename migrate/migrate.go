package migrate

import (
    "database/sql"
    "fmt"
)

// Migrate creates the required tables if they do not exist
func CreateTables(db *sql.DB) error {
    compoundTable := `
    CREATE TABLE IF NOT EXISTS compound (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  scale TEXT CHECK(scale IN ('mg', 'ml'))
);
`
    if _, err := db.Exec(compoundTable); err != nil {
        return fmt.Errorf("failed to create compound table: %w", err)
    }

    quantityTable := `
    CREATE TABLE IF NOT EXISTS quantity (
  id TEXT PRIMARY KEY,
  num_of_units INT NOT NULL,
  quantity_per_unit INT NOT NULL
);
`
    if _, err := db.Exec(quantityTable); err != nil {
        return fmt.Errorf("failed to create quantity table: %w", err)
    }


entryTable := `CREATE TABLE IF NOT EXISTS entry (
  id TEXT PRIMARY KEY,
  type TEXT NOT NULL CHECK(type IN ('incoming', 'outgoing')),
  compound_id TEXT NOT NULL,
  date DATE NOT NULL,
  remark TEXT,
  voucher_no TEXT,
  quantity_id TEXT NOT NULL,
  net_stock INT NOT NULL,
  FOREIGN KEY(compound_id) REFERENCES compound(id),
  FOREIGN KEY(quantity_id) REFERENCES quantity(id)
);
`
    if _, err := db.Exec(entryTable); err != nil {
        return fmt.Errorf("failed to create entry table: %w", err)
    }
    return nil
}

// DropTables drops the `chemicals` and `transactions` tables
func DropTables(db *sql.DB) error {
    if _, err := db.Exec("DROP TABLE IF EXISTS compound;"); err != nil {
        return fmt.Errorf("failed to drop compound table: %w", err)
    }
    if _, err := db.Exec("DROP TABLE IF EXISTS quantity;"); err != nil {
        return fmt.Errorf("failed to drop quantity table: %w", err)
    }
    if _, err := db.Exec("DROP TABLE IF EXISTS entry;"); err != nil {
        return fmt.Errorf("failed to drop entry table: %w", err)
    }
    return nil
}
