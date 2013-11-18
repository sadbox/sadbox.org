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
	postsByMinute = `select count from (select HOUR(Time)*60+MINUTE(Time) as` +
		` date, count(RID)/(SELECT DATEDIFF(NOW(), (SELECT MIN(Time)` +
		` from messages where channel = '#geekhack'))) as count from` +
		` messages where channel ='#geekhack' group by date order by date) as subquery;`
	updateWords = `REPLACE INTO %[1]s
    select newfucks.Nick, newfucks.Posts + (select COALESCE(%[1]s.Posts, 0)), NOW() from 
    (select Nick, SUM(Posts) as Posts from (select Nick, %[2]s as Posts from messages where channel = '#geekhack' and Time > (SELECT COALESCE((select MAX(Updated) from %[1]s), (select MIN(Time) from messages)))) as blah group by Nick having Posts > 0) as newfucks
    LEFT OUTER JOIN 
    %[1]s
    ON newfucks.Nick = %[1]s.Nick;`
	topTenWords = `select Nick, Posts from %s order by Posts desc limit 10;`
)

var (
	geekhack *Geekhack
	logStart = time.Date(2012, 12, 17, 4, 4, 0, 0, time.UTC)
)

type Geekhack struct {
	updateChan chan bool
	db         *sql.DB

	mutex         sync.RWMutex // Protects:
	PostsByDay    []Tuple
	CurseWords    map[string][]Tuple
	TotalPosts    []Tuple
	PostsByMinute []float64
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
		CurseWords: make(map[string][]Tuple),
		updateChan: make(chan bool, 3),
		db:         db,
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
		_, err := g.db.Exec(fmt.Sprintf(updateWords, word.Table, word.BuiltQuery))
		if err != nil {
			return nil, err
		}
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
	PostsByMinute := []float64{}
	rows, err := g.db.Query(postsByMinute)
	if err != nil {
		log.Println(err)
		return
	}
	for rows.Next() {
		var posts float64
		rows.Scan(&posts)
		PostsByMinute = append(PostsByMinute, posts)
	}
	PostsByMinute = movingAverage(PostsByMinute, 10)
	log.Println("PostsByMinute updated in:", time.Since(start))

	// Update the struct
	g.mutex.Lock()
	g.PostsByDay = PostsByDay
	g.TotalPosts = TotalPosts
	g.CurseWords = CurseWords
	g.PostsByMinute = PostsByMinute
	g.age = time.Now()
	g.mutex.Unlock()
	// Finish update, need to unlock it
}

func movingAverage(input []float64, size int) []float64 {
	var start, end int
	result := make([]float64, len(input))
	for i := 0; i < len(input); i++ {
		if i < size {
			start = 0
		} else {
			start = i - size
		}
		if size+i > len(input) {
			end = len(input)
		} else {
			end = size + i
		}
		for _, value := range input[start:end] {
			result[i] += value
		}
		result[i] /= float64(len(input[start:end]))
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
