package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const (
	postByDay = `select DATE(Time) as date, count(RID) as count from messages` +
		` where channel = ? group by date order by count` +
		` desc limit 10;`
	postByDayAll = `select DATE(Time) as date, count(RID) as count from messages` +
		` where channel = ? group by date order by date`
	totalPosts = `select Nick, COUNT(Nick) as Posts from messages where channel` +
		` = ? group by nick order by Posts desc limit 10;`
	postsByMinute = `select count from (select HOUR(Time)*60+MINUTE(Time) as` +
		` date, count(RID)/(SELECT DATEDIFF(NOW(), (SELECT MIN(Time)` +
		` from messages where channel = ?))) as count from` +
		` messages where channel = ? group by date order by date) as subquery;`
	topTenWords = `select Nick, ` + "`" + `%[1]s` + "`" + " from `%[2]s_words` order by " + "`" + `%[1]s` + "`" + ` desc limit 10;`
)

type Geekhack struct {
	Channel  string
	basePath string
	pbmPath  string
	pbdaPath string

	mutex                 sync.RWMutex // Protects:
	PostsByDay            []Tuple
	CurseWords            map[string][]Tuple
	TotalPosts            []Tuple
	PostsByMinute         []float64
	PostsByMinuteSmoothed []float64
	PostByDayAll          [][]int64
	PostByDayAllSmoothed  [][]int64
	age                   time.Time
}

type Tuple struct {
	Name  string
	Count int
}

func NewIRCChannel(channel string) (*Geekhack, error) {
	basePath := fmt.Sprintf("/%s/", strings.Trim(channel, "#"))

	geekhack := &Geekhack{
		Channel:    channel,
		basePath:   basePath,
		pbmPath:    basePath + "postsbyminute",
		pbdaPath:   basePath + "postsbydayall",
		CurseWords: make(map[string][]Tuple),
	}

	go geekhack.Update()
	go geekhack.Updater()

	return geekhack, nil
}

func (g *Geekhack) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case g.basePath:
		g.Main(w, r)
	case g.pbmPath:
		g.pbmHandler(w, r)
	case g.pbdaPath:
		g.pbdaHandler(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (g *Geekhack) Main(w http.ResponseWriter, r *http.Request) {
	g.mutex.RLock()
	defer g.mutex.RUnlock()
	ctx := NewContext(r)
	ctx.Geekhack = g
	if err := templates.ExecuteTemplate(w, "geekhack", ctx); err != nil {
		log.Println(err)
	}
}

func (g *Geekhack) pbmHandler(w http.ResponseWriter, r *http.Request) {
	g.mutex.RLock()
	defer g.mutex.RUnlock()
	type returnStruct struct {
		Type string    `json:"type,omitempty"`
		Name string    `json:"name"`
		Data []float64 `json:"data"`
	}

	jsonSource := []returnStruct{
		returnStruct{
			"",
			"Posts Per Minute",
			g.PostsByMinute,
		},
		returnStruct{
			"spline",
			"Posts Per Minute Smoothed",
			g.PostsByMinuteSmoothed,
		},
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
	type returnStruct struct {
		Type string    `json:"type,omitempty"`
		Name string    `json:"name"`
		Data [][]int64 `json:"data"`
	}
	jsonSource := []returnStruct{
		returnStruct{
			Type: "",
			Name: "Posts Per Day All",
			Data: g.PostByDayAll,
		},
		returnStruct{
			Type: "spline",
			Name: "Posts Per Day All Smoothed",
			Data: g.PostByDayAllSmoothed,
		},
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

func (g *Geekhack) runQuery(query string, insertChannel bool) ([]Tuple, error) {
	var nick string
	var posts int
	var tuple []Tuple
	var rows *sql.Rows
	var err error
	if insertChannel {
		rows, err = sadboxDB.Query(query, g.Channel)
	} else {
		rows, err = sadboxDB.Query(query)
	}
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
	query, err := sadboxDB.Query(`SELECT * FROM words LIMIT 1`)
	if err != nil {
		return nil, err
	}
	defer query.Close()
	words, err := query.Columns()
	if err != nil {
		return nil, err
	}
	for _, word := range words {
		if word == "Nick" {
			continue
		}
		topTenWordsQuery := fmt.Sprintf(topTenWords, word, g.Channel)
		tuple, err := g.runQuery(topTenWordsQuery, false)
		if err != nil {
			return nil, err
		}
		CurseWords[word] = tuple
	}
	return CurseWords, nil
}

func (g *Geekhack) UpdatePostByDayAll() ([][]int64, error) {
	var date string
	var posts int
	var returnValue [][]int64
	rows, err := sadboxDB.Query(postByDayAll, g.Channel)
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
	PostsByDay, err := g.runQuery(postByDay, true)
	if err != nil {
		log.Println(err)
		return
	}

	TotalPosts, err := g.runQuery(totalPosts, true)
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
	rows, err := sadboxDB.Query(postsByMinute, g.Channel, g.Channel)
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
	PostsByMinuteSmoothed := movingAverage(movingAverage(movingAverage(PostsByMinute, 20), 10), 5)
	log.Println("PostsByMinute updated in:", time.Since(start))

	start = time.Now()
	PostsByDayAll, err := g.UpdatePostByDayAll()
	if err != nil {
		log.Println(err)
		return
	}
	PostsByDayAllSmoothed := averageWithTime(PostsByDayAll)
	log.Println("PostsByDayAll updated in:", time.Since(start))

	// Update the struct
	g.mutex.Lock()
	g.PostsByDay = PostsByDay
	g.TotalPosts = TotalPosts
	g.CurseWords = CurseWords
	g.PostsByMinute = PostsByMinute
	g.PostsByMinuteSmoothed = PostsByMinuteSmoothed
	g.PostByDayAll = PostsByDayAll
	g.PostByDayAllSmoothed = PostsByDayAllSmoothed
	g.age = time.Now()
	g.mutex.Unlock()
	// Finish update, need to unlock it
}

func averageWithTime(original [][]int64) [][]int64 {
	first := make([]int64, len(original))
	second := make([]float64, len(original))
	for key, value := range original {
		first[key] = value[0]
		second[key] = float64(value[1])
	}
	second = movingAverage(movingAverage(movingAverage(second, 20), 10), 5)
	result := make([][]int64, len(original))
	for i := 0; i < len(original); i++ {
		result[i] = []int64{first[i], int64(second[i])}
	}
	return result
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
	ticker := time.Tick(2 * time.Minute)
	for _ = range ticker {
		g.Update()
	}
}
