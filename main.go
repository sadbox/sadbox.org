package main

import (
	"encoding/json"
	"encoding/xml"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"path"
	"strings"
)

var (
	templates = template.Must(template.ParseFiles(getFiles("./views/", ".html")...))
	config    Config
)

type Config struct {
	DBConn   string
	Listen   string
	BadWords []BadWord `xml:">BadWord"`
}

type BadWord struct {
	Word  string
	Query string
	Table string
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
		geekhack.updateChan <- true
	}()
	if err := templates.ExecuteTemplate(w, "geekhack.html", geekhack); err != nil {
		log.Println(err)
	}
}
func pbmHandler(w http.ResponseWriter, r *http.Request) {
	geekhack.mutex.RLock()
	defer func() {
		geekhack.mutex.RUnlock()
		geekhack.updateChan <- true
	}()
	jsonSource := struct {
		Name string `json:"name"`
		Data []int  `json:"data"`
	}{
		"Posts By Minute",
		geekhack.PostsByMinute,
	}
	jsonData, err := json.Marshal(jsonSource)
	if err != nil {
		log.Println(err)
		http.Error(w, "Error generating minute data", 500)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
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
	xml.Unmarshal(xmlFile, &config)

	log.Println("Starting sadbox.org on", config.Listen)

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
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static/"))))
	http.ListenAndServe(config.Listen, Log(http.DefaultServeMux))
}
