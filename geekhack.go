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
	postsByMinute = `select count from (select HOUR(Time)*60+MINUTE(Time) as date,` +
		` count(RID) as count from messages where channel = '#geekhack'` +
		` group by date order by date) as subquery;`
	updateWords = `REPLACE INTO %[1]s
    select newfucks.Nick, newfucks.Posts + (select COALESCE(%[1]s.Posts, 0)), NOW() from 
    (select Nick, SUM(Posts) as Posts from (select Nick, ROUND((LENGTH(message) - LENGTH(REPLACE(LOWER(message), "%[2]s", "")))/LENGTH("%[2]s")) as Posts from messages where channel = '#geekhack' and Time > (SELECT COALESCE((select MAX(Updated) from %[1]s), (select MIN(Time) from messages)))) as blah group by Nick having Posts > 0) as newfucks
    LEFT OUTER JOIN 
    %[1]s
    ON newfucks.Nick = %[1]s.Nick;`
	topTenWords = `select Nick, Posts from %s order by Posts desc limit 10;`
)

var geekhack *Geekhack

type Geekhack struct {
	updateChan chan bool
	db         *sql.DB

	mutex         sync.RWMutex // Protects:
	PostsByDay    []Tuple
	CurseWords    map[string][]Tuple
	TotalPosts    []Tuple
	PostsByMinute []int
	age           time.Time
}

type Tuple struct {
	Name  string
	Count int
}

func NewGeekhack() *Geekhack {
	db, err := sql.Open("mysql", config.DBConn)
	if err != nil {
		panic(err)
	}

	return &Geekhack{
		CurseWords:    make(map[string][]Tuple),
		updateChan:    make(chan bool),
		db:            db,
		PostsByMinute: []int{1, 2, 3},
	}
}

func (g *Geekhack) shouldUpdate() bool {
	g.mutex.RLock()
	defer g.mutex.RUnlock()
	return time.Since(g.age).Minutes() > 2
}

func (g *Geekhack) runQuery(query string) ([]Tuple, error) {
	var nick string
	var posts int
	var tuple []Tuple
	rows, err := g.db.Query(query)
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

func (g *Geekhack) UpdateCurseWords() (map[string][]Tuple, error) {
	CurseWords := make(map[string][]Tuple)
	for _, word := range config.BadWords {
		_, err := g.db.Exec(fmt.Sprintf(updateWords, word.Table, word.Query))
		if err != nil {
			return nil, err
		}
		// This is dumb. Either that or I'm too dumb to figure out how to get
		// the sql.Query() thing to allow wildcards. Maybe it's like that by design?
		tuple, err := g.runQuery(fmt.Sprintf(topTenWords, word.Table))
		if err != nil {
			return nil, err
		}
		CurseWords[word.Word] = tuple
	}
	return CurseWords, nil
}

func (g *Geekhack) Update() {
	start := time.Now()
	PostsByDay, err := g.runQuery(postByDay)
	if err != nil {
		log.Println(err)
		return
	}

	TotalPosts, err := g.runQuery(totalPosts)
	if err != nil {
		log.Println(err)
		return
	}
	log.Println("Posts updated in:", time.Since(start))

	start = time.Now()
	CurseWords, err := g.UpdateCurseWords()
	if err != nil {
		log.Println(err)
		return
	}
	log.Println("CurseWords updated in:", time.Since(start))

	start = time.Now()
	PostsByMinute := []int{}
	rows, err := g.db.Query(postsByMinute)
	if err != nil {
		log.Println(err)
		return
	}
	for rows.Next() {
		var posts int
		rows.Scan(&posts)
		PostsByMinute = append(PostsByMinute, posts)
	}
	log.Println("PostsByMinute updated in:", time.Since(start))

	// Update the struct
	g.mutex.Lock()
	g.PostsByDay = PostsByDay
	g.TotalPosts = TotalPosts
	g.CurseWords = CurseWords
	g.PostsByMinute = lowPass(PostsByMinute)
	g.age = time.Now()
	g.mutex.Unlock()
	// Finish update, need to unlock it
}

func lowPass(data []int) []int {
	result := make([]int, len(data))
	result[0] = data[0]
	for i := 1; i < len(data); i++ {
		result[i] = int(float64(result[i-1]) + 0.15*float64(data[i]-result[i-1]))
	}
	return result
}

func (g *Geekhack) Updater() {
	for <-g.updateChan {
		if g.shouldUpdate() {
			g.Update()
		}
	}
}
