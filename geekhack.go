package main

import (
	"database/sql"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"log"
	"sync"
	"time"
)

const (
	postByDay = `select DATE(Time) as date, count(RID) as count from messages` +
		` where channel = '#geekhack' group by date order by count` +
		` desc limit 10;`
	totalPosts = `select Nick, COUNT(Nick) as Posts from messages where channel` +
		` = '#geekhack' group by nick order by Posts desc limit 10;`
	postsByHour = `select HOUR(Time) as date, count(RID) as count from messages` +
		` where channel = '#geekhack' group by date order by date;`
	updateWords = `REPLACE INTO %s
    select newfucks.Nick, newfucks.Posts + %s.Posts, NOW() from 
    (select Nick, COUNT(Nick) as Posts from messages where LOWER(message) like '%%%s%%' and channel = '#geekhack' and Time > (select MAX(Updated) from %s) group by Nick) as newfucks
    LEFT OUTER JOIN 
    %s
    ON newfucks.Nick = %s.Nick;`
	topTenWords = `select Nick, Posts from %s order by Posts desc limit 10;`
)

var geekhack = NewGeekhack()

type Geekhack struct {
	updateChan chan bool

	mutex      sync.RWMutex // Protects:
	PostsByDay []Tuple
	CurseWords map[string][]Tuple
	TotalPosts []Tuple
	age        time.Time
}

type Tuple struct {
	Name  string
	Count int
}

func NewGeekhack() *Geekhack {
	log.Println("Building new geekhack thingy")
	return &Geekhack{
		CurseWords: make(map[string][]Tuple),
		updateChan: make(chan bool),
	}
}

func (g *Geekhack) shouldUpdate() bool {
	return true
	g.mutex.RLock()
	defer g.mutex.RUnlock()
	return time.Since(g.age).Minutes() < 5
}

func runQuery(query string, db *sql.DB) ([]Tuple, error) {
	var nick string
	var posts int
	var tuple []Tuple
	rows, err := db.Query(query)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	for rows.Next() {
		rows.Scan(&nick, &posts)
		tuple = append(tuple, Tuple{nick, posts})
	}
	return tuple, nil
}

func (g *Geekhack) Update() {
	start := time.Now()
	log.Println("Updating GH stats.")
	db, err := sql.Open("mysql", config.DBConn)
	if err != nil {
		log.Println(err)
		return
	}
	defer db.Close()

	PostsByDay, err := runQuery(postByDay, db)
	if err != nil {
		log.Println(err)
		return
	}

	TotalPosts, err := runQuery(totalPosts, db)
	if err != nil {
		log.Println(err)
		return
	}

	CurseWords := make(map[string][]Tuple)
	log.Println("Loading Cursewords!")
	for _, word := range config.BadWords {
		log.Println(word.Word, ":", word.Query)
		_, err := db.Exec(fmt.Sprintf(updateWords, word.Table, word.Table, word.Query, word.Table, word.Table, word.Table))
		if err != nil {
			log.Println(err)
			return
		}
		// This is dumb. Either that or I'm too dumb to figure out how to get
		// the sql.Query() thing to allow wildcards. Maybe it's like that by design?
		tuple, err := runQuery(fmt.Sprintf(topTenWords, word.Table), db)
		if err != nil {
			log.Println(err)
			return
		}
		CurseWords[word.Word] = tuple
	}
	log.Println("Time to import data: ", time.Since(start))
	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.PostsByDay = PostsByDay
	g.TotalPosts = TotalPosts
	g.CurseWords = CurseWords
}

func (g *Geekhack) Updater() {
	for <-g.updateChan {
		if g.shouldUpdate() {
			g.Update()
		}
	}
}
