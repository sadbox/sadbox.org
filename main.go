package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
    "os"
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
	BadWords []struct {
		Word       string
		Query      []string
		Table      string
		BuiltQuery string
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

func Log(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        ForwardedFor := r.Header.Get("X-Forwarded-For")
        if ForwardedFor == "" {
            log.Printf("%s %s %s", r.RemoteAddr, r.Method, r.URL)
        } else {
            log.Printf("%s %s %s", ForwardedFor, r.Method, r.URL)
        }
		handler.ServeHTTP(w, r)
	})
}

func main() {
    configfile, err := os.Open("config.json")
    if err != nil {
        log.Fatal(err)
    }
    decoder := json.NewDecoder(configfile)
    err = decoder.Decode(&config)
    if err != nil {
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

	// These files have to be here
	http.HandleFunc("/favicon.ico", serveStatic("./static/favicon.ico"))
	http.HandleFunc("/sitemap.xml", serveStatic("./static/sitemap.xml"))

	// The plain-jane stuff I serve up
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		if err := templates.ExecuteTemplate(w, "main.html", nil); err != nil {
			log.Println(err)
		}
	})
	http.HandleFunc("/keyboards", keyboardHandler)

	// Geekhack stats! the geekhack struct will handle the routing to sub-things
	http.Handle("/geekhack/", geekhack)
	// Redirects to the right URL so I don't break old links
	http.Handle("/ghstats", http.RedirectHandler("/geekhack/", http.StatusMovedPermanently))
	http.Handle("/geekhack", http.RedirectHandler("/geekhack/", http.StatusMovedPermanently))

	// The rest of the static files
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static/"))))

	log.Fatal(http.ListenAndServe(config.Listen, Log(http.DefaultServeMux)))
}
