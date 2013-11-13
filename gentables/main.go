package main

import (
    "encoding/xml"
    "io/ioutil"
    _ "github.com/go-sql-driver/mysql"
    "database/sql"
    "fmt"
)

const (
    baseTable = `CREATE TABLE %s (
    Nick VARCHAR(32),
    Posts INT(32) NOT NULL DEFAULT 0,
    Updated DATETIME,
    primary KEY (Nick));
    `
    genTableData = `INSERT INTO %s (Nick, Posts, Updated) select Nick, COUNT(Nick) as Posts, NOW() from messages where LOWER(message) like '%%%s%%' and channel = '#geekhack' group by Nick;
    `
)

type Config struct {
    DBConn string
    BadWords []BadWord `xml:">BadWord"`
}

type BadWord struct {
    Word string
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
        fmt.Printf(baseTable, word.Table)
        _, err := db.Exec(fmt.Sprintf(baseTable, word.Table))
        if err != nil {
            panic(err)
        }
        fmt.Printf(genTableData, word.Table, word.Query)
        _, err = db.Exec(fmt.Sprintf(genTableData, word.Table, word.Query))
        if err != nil {
            panic(err)
        }
        fmt.Printf(`select * from %s`,  word.Table)
        _, err = db.Exec(fmt.Sprintf(`select * from %s`, word.Table))
        if err != nil {
            panic(err)
        }
    }
}
