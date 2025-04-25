package db

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3"
)

var Conn *sql.DB

// Sets up the database connection and assigns it to the Global "Conn" variable
func SetUpConnection(filepath string) error {
	conn, err := sql.Open("sqlite3", filepath)
	if err != nil {
		return err
	}

	if err := conn.Ping(); err != nil {
		return err
	}

	Conn = conn
	return nil
}
