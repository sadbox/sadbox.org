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
    }
}

func (g *Geekhack) shouldUpdate() bool {
    g.mutex.RLock()
    defer g.mutex.RUnlock()
    return time.Since(g.age).Minutes() > 15
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
    g.mutex.Lock()
    defer g.mutex.Unlock()
    db, err := sql.Open("mysql", "irclogger:irclogger@/irclogs")
    if err != nil {
        log.Println(err)
        return
    }
    defer db.Close()


    g.PostsByDay, err = runQuery(postByDay, db)
    if err != nil {
        log.Println(err)
        return
    }

    g.TotalPosts, err = runQuery(totalPosts, db)
    if err != nil {
        log.Println(err)
        return
    }
    log.Println(config.BadWords)
    for _, word := range config.BadWords {
        log.Println(word)
        // This is dumb.
        g.CurseWords[word.Word], err = runQuery(fmt.Sprintf(numWords, word.Query), db)
        if err != nil {
            log.Println(err)
            return
        }
    }
    log.Println(*g)
}

func (g *Geekhack) Updater() {
    for <-g.updateChan {
        if g.shouldUpdate() {
            g.Update()
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
    http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static/"))))
    http.ListenAndServe(":8080", nil)
}
