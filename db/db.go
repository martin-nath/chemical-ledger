package db

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/mattn/go-sqlite3"
)


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
