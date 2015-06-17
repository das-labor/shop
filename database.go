package main

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"sync"
)

var Database *sql.DB
var DatabaseMutex sync.Mutex

func InitializeDatabase() error {
	var err error

	Database, err = sql.Open("sqlite3", GlobalConfig.Database)

	return err
}
