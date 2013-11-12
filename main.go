package main

import (
    "net/http"
    "log"
    "html/template"
    "io/ioutil"
    "strings"
    "path"
    "time"
    "sync"
    "fmt"
    "database/sql"
    "encoding/xml"
    _ "github.com/go-sql-driver/mysql"
)

const (
    postByDay = `select DATE(Time) as date, count(RID) as count from messages`+
                ` where channel = '#geekhack' group by date order by count`+
                ` desc limit 10;`
    numWords = `select Nick, COUNT(Nick) as Fucks from messages where message`+
               ` like LOWER('%%%s%%') and channel = '#geekhack' group by nick `+
               `order by Fucks desc limit 10;`
    totalPosts = `select Nick, COUNT(Nick) as Posts from messages where channel`+
                 ` = '#geekhack' group by nick order by Posts desc limit 10;`
    postsByHour = `select HOUR(Time) as date, count(RID) as count from messages`+
                  ` where channel = '#geekhack' group by date order by date;`
)

var (
    templates = template.Must(template.ParseFiles(getFiles("./views/", ".html")...))
    geekhack = NewGeekhack()
    config Config
)

type Config struct {
    DBConn string
    BadWords []BadWord `xml:">BadWord"`
}

type BadWord struct {
    Word string
    Query string
}

type Geekhack struct {
    updateChan chan bool

    mutex sync.RWMutex // Protects:
    PostsByDay []Tuple
    CurseWords map[string][]Tuple
    TotalPosts []Tuple
    age time.Time
}

type Tuple struct {
    Name string
    Count int
}

func NewGeekhack() *Geekhack{
    return &Geekhack{
        CurseWords: make(map[string][]Tuple),
        updateChan: make(chan bool),
    }
}

func (g *Geekhack) shouldUpdate() bool {
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
        // This is dumb. Either that or I'm too dumb to figure out how to get
        // the sql.Query() thing to allow wildcards. Maybe it's like that by design?
        tuple, err := runQuery(fmt.Sprintf(numWords, word.Query), db)
        if err != nil {
            log.Println(err)
            return
        }
        CurseWords[word.Word] = tuple
    }
    log.Println("Finished loading Cursewords!")
    g.mutex.Lock()
    defer g.mutex.Unlock()
    g.PostsByDay = PostsByDay
    g.TotalPosts = TotalPosts
    g.CurseWords = CurseWords
}

func (g *Geekhack) Updater() {
    log.Println("I'm waiting for stuff and junk!")
    for <-g.updateChan {
        log.Println("I found stuff and junk!")
        if g.shouldUpdate() {
            g.Update()
            log.Println("Stuff and junk complete!")
        }
    }
}

func getFiles(folder, fileType string) []string {
    files, err := ioutil.ReadDir(folder)
    if err != nil {
        panic(err)
    }
    var templateList []string
    for _, file := range files {
        if strings.HasSuffix(file.Name(), fileType) {
            templateList = append(templateList, folder+file.Name())
        }
    }
    return templateList
}

func serveTemplate(filename string) func(http.ResponseWriter, *http.Request) {
    return func(w http.ResponseWriter, r *http.Request) {
        if err := templates.ExecuteTemplate(w, filename, nil); err != nil {
            log.Println(err)
        }
    }
}

type Keyboards struct {
    Keyboards map[string]string
}

func keyboardHandler(w http.ResponseWriter, r *http.Request) {
    keyboards := getFiles("./static/keyboards/", ".jpg")
    matchedBoards := Keyboards{make(map[string]string)}
    for _, keyboard := range keyboards {
        dir, file := path.Split(keyboard)
        matchedBoards.Keyboards[path.Join("/", dir, file)] = path.Join("/", dir, "thumbs", file)
    }
    if err := templates.ExecuteTemplate(w, "keyboards.html", matchedBoards); err != nil {
        log.Println(err)
    }
}

func geekhackHandler(w http.ResponseWriter, r *http.Request) {
    geekhack.mutex.RLock()
    defer func() {
        geekhack.mutex.RUnlock()
        log.Println("Chillin' on channel!")
        geekhack.updateChan <- true
        log.Println("Shit's cash.")
    }()
    if err := templates.ExecuteTemplate(w, "geekhack.html", geekhack); err != nil {
        log.Println(err)
    }
}

func serveStatic(filename string) func(http.ResponseWriter, *http.Request) {
    return func(w http.ResponseWriter, r *http.Request) {
        http.ServeFile(w, r, filename)
    }
}

func init() {
    xmlFile, err := ioutil.ReadFile("config.xml")
    if err != nil {
        log.Fatal(err)
    }
    xml.Unmarshal(xmlFile, &config)
    log.Println(config)
    start := time.Now()
    geekhack.Update()
    log.Println("Time to import data: ", time.Since(start))
    go geekhack.Updater()
}

func main() {
    http.HandleFunc("/favicon.ico", serveStatic("./favicon.ico"))
    http.HandleFunc("/sitemap.xml", serveStatic("./sitemap.xml"))
    http.HandleFunc("/", serveTemplate("main.html"))
    http.HandleFunc("/status", serveTemplate("status.html"))
    http.HandleFunc("/keyboards", keyboardHandler)
    http.HandleFunc("/geekhack", geekhackHandler)
    http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static/"))))
    http.ListenAndServe(":8080", nil)
}
