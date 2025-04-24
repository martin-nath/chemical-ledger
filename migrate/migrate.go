package migrate

import (
	"database/sql"
	"fmt"
	"os"
	"sync"

	_ "embed"
)

//go:embed create-tables.sql
var createTableQuery string

func CreateTables(db *sql.DB) error {
	insertCompoundsQuery := ""
	errCh := make(chan error, 1)
	wg := sync.WaitGroup{}
	wg.Add(1)

	go func(insertCompoundsQuery *string) {
		defer wg.Done()

		// Not using go:embed because we don't want to embed the file in the binary.
		// Instead, we read it from the file system, which allows us to change the file without rebuilding the binary.
		query, err := os.ReadFile("insert-compounds.sql")
		if err != nil {
			errCh <- fmt.Errorf("failed to read insert-compounds.sql: %w", err)
		}
		*insertCompoundsQuery = string(query)
	}(&insertCompoundsQuery)

	if _, err := db.Exec(`DROP TABLE IF EXISTS compound`); err != nil {
		return fmt.Errorf("failed to drop compound table: %w", err)
	}

	if _, err := db.Exec(createTableQuery); err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	wg.Wait()
	close(errCh)
	if err := <-errCh; err != nil {
		return err
	}

	if insertCompoundsQuery == "" {
		return fmt.Errorf("file insert-compounds.sql not found")
	}

	if _, err := db.Exec(insertCompoundsQuery); err != nil {
		return fmt.Errorf("failed to insert compounds: %w", err)
	}
	return nil
}
