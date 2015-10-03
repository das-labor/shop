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

	if err != nil {
		return err
	}

	InitializeSchema()

	return nil
}

func InitializeSchema() {
	Database.Exec("CREATE TABLE products (id INTEGER PRIMARY KEY, name STRING, slug STRING, description STRING, price INTEGER, count INTEGER)")
	Database.Exec("CREATE TABLE members (id INTEGER PRIMARY KEY, name STRING UNIQUE, email STRING, passwd STRING, grp STRING)")
	Database.Exec("CREATE TABLE sessions (id STRING PRIMARY KEY, member INTEGER, lastseen INTEGER)")
	Database.Exec("CREATE TABLE carts (product INTEGER, session STRING, count INTEGER)")
}
