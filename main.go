package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
)

type Configuration struct {
	Location     string // Base Url
	Listen       string // URL to listen on
	CookieDomain string
	Salt         string // Salt used for password hashing
	Templates    string // Path to the template dir
	Database     string // Path to the SQLite database
}

const Version string = "0.1"
const ConfigFile string = "config.json"

var GlobalConfig Configuration

func main() {
	fmt.Println(`LABOR Shop  Copyright (C) 2015 Kai Michaelis
This program comes with ABSOLUTELY NO WARRANTY.
This is free software, and you are welcome to redistribute it
under certain conditions.`)

	cfgfile, err := os.Open(ConfigFile)
	if err != nil {
		log.Fatal(err)
	}

	cfgenc := json.NewDecoder(cfgfile)

	err = cfgenc.Decode(&GlobalConfig)
	if err != nil {
		log.Fatal(err)
	}

	err = InitializeTemplates()
	if err != nil {
		log.Fatal(err)
	}

	err = InitializeDatabase()
	if err != nil {
		log.Fatal(err)
	}

	err = InitializeRoutes()
	if err != nil {
		log.Fatal(err)
	}
	http.ListenAndServe(GlobalConfig.Listen, nil)
}
