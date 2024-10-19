package main

import (
	"crypto/rand"
	"crypto/tls"
	"database/sql"
	"fmt"
	"html/template"
	"log"
	mathRand "math/rand"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/klauspost/compress/gzhttp"
	"github.com/mattn/go-sqlite3"
	"github.com/rs/zerolog"
	sqldblogger "github.com/simukti/sqldb-logger"
	"github.com/simukti/sqldb-logger/logadapter/zerologadapter"
)

var logger = log.New(os.Stdout, "sadbox.org: ", log.Ldate|log.Ltime|log.Lshortfile)

var templates = template.New("").Funcs(template.FuncMap{"add": func(a, b int) int { return a + b }})

var hostname_whitelist = []string{
	"www.sadbox.org", "sadbox.org", "mail.sadbox.org",
	"www.sadbox.es", "sadbox.es",
	"www.geekwhack.org", "geekwhack.org",
	"www.sadbox.xyz", "sadbox.xyz",
}

var sadboxDB *sql.DB

var channels []channel

type channel struct {
	LinkName    string
	ChannelName string
}

type WebsiteName struct {
	Title, Brand string
}

type Main struct {
	Channels []channel
}

type TemplateContext struct {
	Geekhack *Geekhack
	Webname  *WebsiteName
	Main     *Main
}

func NewContext(r *http.Request, appendToTitle string) *TemplateContext {
	title := "sadbox \u00B7 org"
	host := "sadbox.org"
	for _, v := range hostname_whitelist {
		if v == r.Host {
			trimmed := strings.TrimPrefix(r.Host, "www.")
			title = strings.Replace(trimmed, ".", " \u00B7 ", -1)
			host = trimmed
			break
		}
	}
	return &TemplateContext{
		Webname: &WebsiteName{
			title + " - " + appendToTitle,
			host,
		},
	}
}

func CatchPanic(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if r := recover(); r != nil {
				logger.Printf("Recovered from panic: %v", r)
				http.Error(w, "Something went wrong!", http.StatusInternalServerError)
			}
		}()

		handler.ServeHTTP(w, r)
	})
}

func SendToHTTPS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Connection", "close")
	http.Redirect(w, r, "https://"+r.Host+r.RequestURI, http.StatusMovedPermanently)
}

func RedirectToHTTPS(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Println(r.Host)

		// Don't bother sending IPs to https
		ip := net.ParseIP(strings.Trim(r.Host, "[]"))
		if ip != nil {
			handler.ServeHTTP(w, r)
			return
		}

		// If r.Host isn't an ip, maybe it's an ip:port?
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			SendToHTTPS(w, r)
			return
		}
		if ip = net.ParseIP(host); ip == nil { // Couldn't parse the split ip
			SendToHTTPS(w, r)
			return
		}

		handler.ServeHTTP(w, r)
	})
}

func AddHeaders(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=120")
		if r.TLS != nil {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
		}
		w.Header().Set("Content-Security-Policy", "default-src 'self'; object-src 'none'; frame-ancestors 'none'")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer, same-origin")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		handler.ServeHTTP(w, r)
	})
}

func Log(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		remoteHost := r.Header.Get("X-Forwarded-For")
		if remoteHost == "" {
			remoteHost = r.RemoteAddr
		}
		logger.Printf("%s %s %s %s", remoteHost, r.Host, r.Method, r.URL)
		handler.ServeHTTP(w, r)
	})
}

type sessionKeys struct {
	keys [][32]byte
	buf  []byte
	tls  *tls.Config
}

func NewSessionKeys(tls *tls.Config) *sessionKeys {
	sk := &sessionKeys{
		buf: make([]byte, 32),
		tls: tls,
	}
	sk.addNewKey()
	return sk
}

func (sk *sessionKeys) addNewKey() {
	rand.Read(sk.buf)
	newKey := [32]byte{}
	copy(newKey[:], sk.buf)
	sk.keys = append([][32]byte{newKey}, sk.keys...)
	if len(sk.keys) > 4 {
		sk.keys = sk.keys[:5]
	}
	logger.Println("Rotating Session Keys")
	sk.tls.SetSessionTicketKeys(sk.keys)
}

func (sk *sessionKeys) Spin() {
	for range time.Tick(24 * time.Hour) {
		sk.addNewKey()
	}
}

func main() {
	logger.Println("Starting sadbox.org")

	template.Must(templates.ParseGlob("/views/*.tmpl"))
	logger.Println(templates.DefinedTemplates())

	var err error
	dsn := "file:/db/sadbot_archive.db"
	loggerAdapter := zerologadapter.New(zerolog.New(os.Stdout))
	sadboxDB = sqldblogger.OpenDriver(dsn, &sqlite3.SQLiteDriver{}, loggerAdapter)
	defer sadboxDB.Close()

	err = sadboxDB.Ping()
	if err != nil {
		logger.Fatal(err)
	}

	staticFileServer := http.FileServer(http.Dir("/static-files"))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.Host, "geekwhack.org") {
			fmt.Fprintf(w, "rip ripster55\n")
		} else if r.URL.Path == "/" {
			ctx := NewContext(r, "home")
			ctx.Main = &Main{channels}
			if err := templates.ExecuteTemplate(w, "main.tmpl", ctx); err != nil {
				logger.Println(err)
			}
		} else {
			staticFileServer.ServeHTTP(w, r)
		}
	})

	mathRand.Seed(time.Now().UnixNano())

	rows, err := sadboxDB.Query(`SELECT Channel from Channels;`)
	if err != nil {
		logger.Fatal(err)
	}
	for rows.Next() {
		var channelName string
		err := rows.Scan(&channelName)
		if err != nil {
			logger.Fatal(err)
		}

		chanInfo := channel{fmt.Sprintf("/%s/", strings.Trim(channelName, "#")), channelName}

		ircChanHandler, err := NewIRCChannel(chanInfo)
		if err != nil {
			logger.Fatal(err)
		}
		http.Handle(chanInfo.LinkName, ircChanHandler)

		channels = append(channels, chanInfo)
	}

	// Redirects to the right URL so I don't break old links
	http.Handle("/ghstats", http.RedirectHandler("/geekhack/", http.StatusMovedPermanently))
	http.Handle("/geekhack", http.RedirectHandler("/geekhack/", http.StatusMovedPermanently))

	// This will redirect people to the gmail page
	http.Handle("mail.sadbox.org/", http.RedirectHandler("https://mail.google.com/a/sadbox.org", http.StatusFound))

	servemux := gzhttp.GzipHandler(
		CatchPanic(
			Log(
				AddHeaders(http.DefaultServeMux))))

	server := &http.Server{
		Addr:              ":9000",
		Handler:           servemux,
		ReadTimeout:       60 * time.Minute,
		ReadHeaderTimeout: 30 * time.Second,
		WriteTimeout:      60 * time.Minute,
		IdleTimeout:       30 * time.Minute,
	}
	logger.Fatal(server.ListenAndServe())
}
