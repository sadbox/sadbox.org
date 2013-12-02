package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"path"
	"regexp"
	"strings"
)

var (
	templates  = template.Must(template.ParseFiles(getFiles("./views/", ".html")...))
	fourOhFour = regexp.MustCompile("^/(status)?$")
	config     Config
)

type Config struct {
	DBConn   string
	Listen   string
	BadWords []BadWord `xml:">BadWord"`
}

type BadWord struct {
	Word       string
	Query      []string
	Table      string
	BuiltQuery string
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
		m := fourOhFour.FindStringSubmatch(r.URL.Path)
		if m == nil {
			http.NotFound(w, r)
			return
		}

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
		select {
		case geekhack.updateChan <- true:
		default:
		}
	}()
	if err := templates.ExecuteTemplate(w, "geekhack.html", geekhack); err != nil {
		log.Println(err)
	}
}

func pbmHandler(w http.ResponseWriter, r *http.Request) {
	geekhack.mutex.RLock()
	defer geekhack.mutex.RUnlock()
	jsonSource := struct {
		Name string    `json:"name"`
		Data []float64 `json:"data"`
	}{
		"Posts Per Minute",
		geekhack.PostsByMinute,
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

func pbdaHandler(w http.ResponseWriter, r *http.Request) {
	geekhack.mutex.RLock()
	defer geekhack.mutex.RUnlock()
	jsonSource := struct {
		Name string    `json:"name"`
		Data [][]int64 `json:"data"`
	}{
		"Posts Per Day All",
		geekhack.PostByDayAll,
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

func serveStatic(filename string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filename)
	}
}

func Log(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s", r.RemoteAddr, r.Method, r.URL)
		handler.ServeHTTP(w, r)
	})
}

func main() {
	xmlFile, err := ioutil.ReadFile("config.xml")
	if err != nil {
		log.Fatal(err)
	}
	if err = xml.Unmarshal(xmlFile, &config); err != nil {
		log.Fatal(err)
	}

	log.Println("Starting sadbox.org on", config.Listen)

	for outerIndex, word := range config.BadWords {
		for innerIndex, searchTerm := range word.Query {
			word.BuiltQuery = word.BuiltQuery + fmt.Sprintf(`ROUND((LENGTH(message) - LENGTH(REPLACE(LOWER(message), "%[1]s", "")))/LENGTH("%[1]s"))`, searchTerm)
			if innerIndex != len(word.Query)-1 {
				word.BuiltQuery = word.BuiltQuery + "+"
			}
		}
		config.BadWords[outerIndex] = word
	}

	geekhack = NewGeekhack()
	defer geekhack.db.Close()

	geekhack.Update()
	go geekhack.Updater()

	http.HandleFunc("/favicon.ico", serveStatic("./favicon.ico"))
	http.HandleFunc("/sitemap.xml", serveStatic("./sitemap.xml"))
	http.HandleFunc("/", serveTemplate("main.html"))
	http.HandleFunc("/status", serveTemplate("status.html"))
	http.HandleFunc("/keyboards", keyboardHandler)
	http.HandleFunc("/geekhack", geekhackHandler)
	http.HandleFunc("/geekhack/postsbyminute", pbmHandler)
	http.HandleFunc("/geekhack/postsbydayall", pbdaHandler)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static/"))))
	log.Fatal(http.ListenAndServe(config.Listen, Log(http.DefaultServeMux)))
}
