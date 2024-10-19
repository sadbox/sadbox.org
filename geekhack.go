package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

const (
	postByDay = `select DATE(Time) as date, count(RID) as count from messages` +
		` where channel = ? group by date order by count` +
		` desc limit 10;`
	postByDayAll = `select DATE(Time) as date, count(RID) as count from messages` +
		` where channel = ? group by date order by date`
	totalPosts = `select Nick, COUNT(Nick) as Posts from messages where channel` +
		` = ? group by nick order by Posts desc limit 10;`
	postsByMinute = `WITH time_table as (select ((CAST(strftime('%J', MAX(Time)) as REAL)` +
		` - CAST(strftime('%J', MIN(Time)) as REAL)) * 1440) time_span_minutes ` +
		`from messages where channel = ?) ` +
		`select messages / time_table.time_span_minutes from ( ` +
		`select COUNT(RID) messages, ` +
		`(CAST(strftime('%H',Time) as INT)*60)+(CAST(strftime('%M',Time) as INT)) ` +
		`minute_timestamp from messages where messages.channel = ? ` +
		`group by minute_timestamp order by minute_timestamp), time_table;`
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
	PostByDayAll          []TimePoint
	PostByDayAllSmoothed  []TimePoint
	age                   time.Time
}

type TimePoint struct {
	X int64 `json:"x"`
	Y int64 `json:"y"`
}

type Tuple struct {
	Name  string
	Count int
}

func NewIRCChannel(chanInfo channel) (*Geekhack, error) {
	geekhack := &Geekhack{
		Channel:    chanInfo.ChannelName,
		basePath:   chanInfo.LinkName,
		pbmPath:    chanInfo.LinkName + "postsbyminute",
		pbdaPath:   chanInfo.LinkName + "postsbydayall",
		CurseWords: make(map[string][]Tuple),
	}

	go geekhack.Update()

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
	ctx := NewContext(r, g.Channel)
	ctx.Geekhack = g
	if err := templates.ExecuteTemplate(w, "geekhack.tmpl", ctx); err != nil {
		log.Println(err)
	}
}

func (g *Geekhack) pbmHandler(w http.ResponseWriter, r *http.Request) {
	g.mutex.RLock()
	defer g.mutex.RUnlock()
	type returnStruct struct {
		Label           string    `json:"label"`
		Data            []float64 `json:"data"`
		Fill            bool      `json:"fill"`
		BackgroundColor string    `json:"backgroundColor"`
		BorderColor     string    `json:"borderColor"`
	}

	jsonSource := []returnStruct{
		returnStruct{
			"Posts Per Minute Smoothed",
			g.PostsByMinuteSmoothed,
			false,
			"#0f172a",
			"#0f172a",
		},
		returnStruct{
			"Posts Per Minute",
			g.PostsByMinute,
			true,
			"#3b82f6",
			"#1e40af",
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
		Label           string      `json:"label"`
		Data            []TimePoint `json:"data"`
		Fill            bool        `json:"fill"`
		BackgroundColor string      `json:"backgroundColor"`
		BorderColor     string      `json:"borderColor"`
	}
	jsonSource := []returnStruct{
		returnStruct{
			Label:           "Posts Per Day All Smoothed",
			Data:            g.PostByDayAllSmoothed,
			Fill:            false,
			BackgroundColor: "#0f172a",
			BorderColor:     "#0f172a",
		},
		returnStruct{
			Label:           "Posts Per Day All",
			Data:            g.PostByDayAll,
			Fill:            true,
			BackgroundColor: "#3b82f6",
			BorderColor:     "#1e40af",
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

func (g *Geekhack) UpdatePostByDayAll() ([]TimePoint, error) {
	var date string
	var posts int
	var returnValue []TimePoint
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
		returnValue = append(returnValue, TimePoint{convTime.UnixMilli(), int64(posts)})
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
		var posts sql.NullFloat64
		if err := rows.Scan(&posts); err != nil {
			log.Println(err)
			return
		}
		if posts.Valid {
			PostsByMinute = append(PostsByMinute, posts.Float64)
		} else {
			PostsByMinute = append(PostsByMinute, float64(0))
		}
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

func averageWithTime(original []TimePoint) []TimePoint {
	first := make([]int64, len(original))
	second := make([]float64, len(original))
	for key, value := range original {
		first[key] = value.X
		second[key] = float64(value.Y)
	}
	second = movingAverage(movingAverage(movingAverage(second, 20), 10), 5)
	result := make([]TimePoint, len(original))
	for i := 0; i < len(original); i++ {
		result[i] = TimePoint{first[i], int64(second[i])}
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
