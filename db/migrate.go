package db

import (
	_ "embed"
	"errors"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed create-tables.sql
var createTablesQuery string

// Create the tables in the database
func CreateTables() error {
	if Conn == nil {
		return errors.New("database connection not set up, run SetUpConnection() first")
	}

	if _, err := Conn.Exec(createTablesQuery); err != nil {
		return err
	}

	return nil
}

// Drop the tables in the database
func DropTables() error {
	if Conn == nil {
		return errors.New("database connection not set up, run SetUpConnection() & CreateTables() first")
	}

	if _, err := Conn.Exec("DROP TABLE IF EXISTS entry"); err != nil {
		return err
	}

	if _, err := Conn.Exec("DROP TABLE IF EXISTS compound"); err != nil {
		return err
	}

	if _, err := Conn.Exec("DROP TABLE IF EXISTS quantity"); err != nil {
		return err
	}

	return nil
}
