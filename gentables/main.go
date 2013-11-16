package main

import (
	"database/sql"
	"encoding/xml"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"io/ioutil"
)

const (
	baseTable = `CREATE TABLE %s (
    Nick VARCHAR(32),
    Posts INT(32) NOT NULL DEFAULT 0,
    Updated DATETIME,
    primary KEY (Nick));
    `
)

type Config struct {
	DBConn   string
	BadWords []BadWord `xml:">BadWord"`
}

type BadWord struct {
	Word  string
	Query string
	Table string
}

func main() {
	var config Config
	xmlFile, err := ioutil.ReadFile("../config.xml")
	if err != nil {
		panic(err)
	}
	xml.Unmarshal(xmlFile, &config)

	db, err := sql.Open("mysql", config.DBConn)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	for _, word := range config.BadWords {
		fmt.Printf(`drop table if exists %s;`, word.Table)
		_, err := db.Exec(fmt.Sprintf(`drop table if exists %s;`, word.Table))
		if err != nil {
			panic(err)
		}
	}
	for _, word := range config.BadWords {
		fmt.Printf(baseTable, word.Table)
		_, err = db.Exec(fmt.Sprintf(baseTable, word.Table))
		if err != nil {
			panic(err)
		}
	}
}
