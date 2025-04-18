package migrate

import (
	"database/sql"
	"fmt"

	_ "embed"
)

//go:embed create-tables.sql
var createTableQuery string

//go:embed insert-compounds.sql
var insertCompoundsQuery string

func CreateTables(db *sql.DB) error {
	if _, err := db.Exec(createTableQuery); err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}
	if _, err := db.Exec(insertCompoundsQuery); err != nil {
		return fmt.Errorf("failed to insert compounds: %w", err)
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
