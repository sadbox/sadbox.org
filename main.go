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
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/daaku/go.httpgzip"
	"golang.org/x/crypto/acme/autocert"
)

//go:generate esc -private -o bindata.go -prefix static ./views ./static/
var templates = template.New("").Funcs(template.FuncMap{"add": func(a, b int) int { return a + b }})
var templateSources = []string{
	"/views/main.tmpl",
	"/views/_util.tmpl",
	"/views/geekhack.tmpl",
}

var hostname_whitelist = []string{
	"www.sadbox.org", "sadbox.org", "mail.sadbox.org",
	"www.sadbox.es", "sadbox.es",
	"www.geekwhack.org", "geekwhack.org",
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

func NewContext(r *http.Request) *TemplateContext {
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
			title,
			host,
		},
	}
}

func CatchPanic(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Recovered from panic: %v", r)
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
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			SendToHTTPS(w, r)
			return
		}

		ip := net.ParseIP(host)
		if ip == nil {
			SendToHTTPS(w, r)
			return
		}

		if !ip.IsLoopback() {
			SendToHTTPS(w, r)
			return
		}

		handler.ServeHTTP(w, r)
	})
}

func AddHeaders(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=120")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
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
		log.Printf("%s %s %s", remoteHost, r.Method, r.URL)
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
	log.Println("Rotating Session Keys")
	sk.tls.SetSessionTicketKeys(sk.keys)
}

func (sk *sessionKeys) Spin() {
	for range time.Tick(24 * time.Hour) {
		sk.addNewKey()
	}
}

func main() {
	log.Println("Starting sadbox.org")

	for _, filename := range templateSources {
		template.Must(templates.Parse(_escFSMustString(false, filename)))
	}

	var err error
	sadboxDB, err = sql.Open("mysql", os.Getenv("GEEKHACK_DB"))
	if err != nil {
		log.Fatal(err)
	}
	defer sadboxDB.Close()

	err = sadboxDB.Ping()
	if err != nil {
		log.Fatal(err)
	}

	staticFileServer := http.FileServer(_escFS(false))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			ctx := NewContext(r)
			ctx.Main = &Main{channels}
			if err := templates.ExecuteTemplate(w, "main", ctx); err != nil {
				log.Println(err)
			}
		} else {
			staticFileServer.ServeHTTP(w, r)
		}
	})

	mathRand.Seed(time.Now().UnixNano())

	rows, err := sadboxDB.Query(`SELECT Channel from Channels;`)
	if err != nil {
		log.Fatal(err)
	}
	for rows.Next() {
		var channelName string
		err := rows.Scan(&channelName)
		if err != nil {
			log.Fatal(err)
		}

		chanInfo := channel{fmt.Sprintf("/%s/", strings.Trim(channelName, "#")), channelName}

		ircChanHandler, err := NewIRCChannel(chanInfo)
		if err != nil {
			log.Fatal(err)
		}
		http.Handle(chanInfo.LinkName, ircChanHandler)

		channels = append(channels, chanInfo)
	}

	// Redirects to the right URL so I don't break old links
	http.Handle("/ghstats", http.RedirectHandler("/geekhack/", http.StatusMovedPermanently))
	http.Handle("/geekhack", http.RedirectHandler("/geekhack/", http.StatusMovedPermanently))

	// This will redirect people to the gmail page
	http.Handle("mail.sadbox.org/", http.RedirectHandler("https://mail.google.com/a/sadbox.org", http.StatusFound))

	localhost_znc, err := url.Parse("http://127.0.0.1:6698")
	if err != nil {
		log.Fatal(err)
	}
	http.Handle("/znc/", httputil.NewSingleHostReverseProxy(localhost_znc))
	http.Handle("/znc", http.RedirectHandler("/znc/", http.StatusMovedPermanently))

	servemux := httpgzip.NewHandler(
		CatchPanic(
			Log(
				AddHeaders(http.DefaultServeMux))))

	httpSrv := &http.Server{
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		Handler:      RedirectToHTTPS(servemux),
	}
	go func() { log.Fatal(httpSrv.ListenAndServe()) }()

	m := autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Cache:      autocert.DirCache("/home/sadbox-web/cert-cache"),
		HostPolicy: autocert.HostWhitelist(hostname_whitelist...),
		Email:      "blue6249@gmail.com",
		ForceRSA:   true,
	}

	var certs []tls.Certificate
	sadboxCert, err := tls.LoadX509KeyPair("/home/sadbox-web/cert-cache/sadbox.org",
		"/home/sadbox-web/cert-cache/sadbox.org")

	if err == nil {
		certs = append(certs, sadboxCert)
	}

	tlsconfig := &tls.Config{
		PreferServerCipherSuites: true,
		GetCertificate:           m.GetCertificate,
		Certificates:             certs,
	}

	go NewSessionKeys(tlsconfig).Spin()

	server := &http.Server{
		Addr:         ":https",
		Handler:      servemux,
		TLSConfig:    tlsconfig,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}
	log.Fatal(server.ListenAndServeTLS("", ""))
}
