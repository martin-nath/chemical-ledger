package db

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"
)

var Db *sql.DB

func InitDB(dataSourceName string) {
	var err error
	Db, err = sql.Open("sqlite3", dataSourceName)
	if err != nil {
		logrus.Fatalf("Failed to open database: %v", err)
		return
	}
	if err = Db.Ping(); err != nil {
		logrus.Fatalf("Failed to connect to database: %v", err)
		return
	}
	logrus.Info("Connected to the database")
}
