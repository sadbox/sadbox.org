package main

import (
	"database/sql"
	"encoding/json"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
	"log"
	"net/http"
	"sync"
	"time"
)

const (
	postByDay = `select DATE(Time) as date, count(RID) as count from messages` +
		` where channel = '#geekhack' group by date order by count` +
		` desc limit 10;`
	postByDayAll = `select DATE(Time) as date, count(RID) as count from messages` +
		` where channel = '#geekhack' group by date order by date`
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

var geekhack *Geekhack

type Geekhack struct {
	updateChan chan bool
	db         *sql.DB

	mutex         sync.RWMutex // Protects:
	PostsByDay    []Tuple
	CurseWords    map[string][]Tuple
	TotalPosts    []Tuple
	PostsByMinute []float64
	PostByDayAll  [][]int64
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

func (g *Geekhack) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/geekhack/":
		g.Main(w, r)
	case "/geekhack/postsbyminute":
		g.pbmHandler(w, r)
	case "/geekhack/postsbydayall":
		g.pbdaHandler(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (g *Geekhack) Main(w http.ResponseWriter, r *http.Request) {
	g.mutex.RLock()
	defer func() {
		g.mutex.RUnlock()
		select {
		case g.updateChan <- true:
		default:
		}
	}()
	if err := templates.ExecuteTemplate(w, "geekhack.html", g); err != nil {
		log.Println(err)
	}
}

func (g *Geekhack) pbmHandler(w http.ResponseWriter, r *http.Request) {
	g.mutex.RLock()
	defer g.mutex.RUnlock()
	jsonSource := struct {
		Name string    `json:"name"`
		Data []float64 `json:"data"`
	}{
		"Posts Per Minute",
		g.PostsByMinute,
	}
	jsonData, err := json.Marshal(jsonSource)
	if err != nil {
		log.Println(err)
		http.Error(w, "Error generating minute data", 500)
	}
	w.Header().Set("Content-Type", "application/json")
	written, err := w.Write(jsonData)
	if written < len(jsonData) || err != nil {
		log.Println("Error writing response to client")
	}
}

func (g *Geekhack) pbdaHandler(w http.ResponseWriter, r *http.Request) {
	g.mutex.RLock()
	defer g.mutex.RUnlock()
	jsonSource := struct {
		Name string    `json:"name"`
		Data [][]int64 `json:"data"`
	}{
		"Posts Per Day All",
		g.PostByDayAll,
	}
	jsonData, err := json.Marshal(jsonSource)
	if err != nil {
		log.Println(err)
		http.Error(w, "Error generating day data", 500)
	}
	w.Header().Set("Content-Type", "application/json")
	written, err := w.Write(jsonData)
	if written < len(jsonData) || err != nil {
		log.Println("Error writing response to client")
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
		if err := rows.Scan(&nick, &posts); err != nil {
			return nil, err
		}
		tuple = append(tuple, Tuple{nick, posts})
	}
	if err := rows.Err(); err != nil {
		return nil, err
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

func (g *Geekhack) UpdatePostByDayAll() ([][]int64, error) {
	var date string
	var posts int
	var returnValue [][]int64
	rows, err := g.db.Query(postByDayAll)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		if err := rows.Scan(&date, &posts); err != nil {
			return nil, err
		}
		convTime, err := time.Parse("2006-01-02", date)
		if err != nil {
			return nil, err
		}
		returnValue = append(returnValue, []int64{convTime.Unix() * 1000, int64(posts)})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return returnValue, nil
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
		if err := rows.Scan(&posts); err != nil {
			log.Println(err)
			return
		}
		PostsByMinute = append(PostsByMinute, posts)
	}
	if err := rows.Err(); err != nil {
		log.Println(err)
		return
	}
	PostsByMinute = movingAverage(PostsByMinute, 10)
	log.Println("PostsByMinute updated in:", time.Since(start))

	start = time.Now()
	PostsByDayAll, err := g.UpdatePostByDayAll()
	if err != nil {
		log.Println(err)
		return
	}
	log.Println("PostsByDayAll updated in:", time.Since(start))

	// Update the struct
	g.mutex.Lock()
	g.PostsByDay = PostsByDay
	g.TotalPosts = TotalPosts
	g.CurseWords = CurseWords
	g.PostsByMinute = PostsByMinute
	g.PostByDayAll = PostsByDayAll
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
		if size+i+1 > len(input) {
			end = len(input)
		} else {
			end = size + i + 1
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
