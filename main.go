package main

import (
	"bytes"
	"crypto/tls"
	"html/template"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"github.com/daaku/go.httpgzip"
	"golang.org/x/crypto/acme/autocert"
)

//go:generate go-bindata ./static/... ./views
var templates = template.New("").Funcs(template.FuncMap{"add": func(a, b int) int { return a + b }})

var hostname_whitelist = []string{
	"www.sadbox.org", "sadbox.org",
	"www.sadbox.es", "sadbox.es",
	"www.geekwhack.org", "geekwhack.org",
}

type WebsiteName struct {
	Title, Brand string
}

type TemplateContext struct {
	Geekhack *Geekhack
	Webname  *WebsiteName
}

func NewContext(r *http.Request) *TemplateContext {
	title := "sadbox \u00B7 org"
	host := "sadbox.org"
	for _, v := range hostname_whitelist {
		if v == r.Host {
			title = strings.Replace(r.Host, ".", " \u00B7 ", -1)
			host = r.Host
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

func serveStatic(filename string) {
	relPath, err := filepath.Rel("static", filename)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Serving Static File:", relPath)

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=31536000")
		asset, err := Asset(filename)
		if err != nil {
			http.NotFound(w, r)
		}
		assetInfo, err := AssetInfo(filename)
		if err != nil {
			http.NotFound(w, r)
		}
		http.ServeContent(w, r, filename, assetInfo.ModTime(), bytes.NewReader(asset))
	}

	http.HandleFunc(path.Join("/", relPath), handler)
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
		w.Header().Set("Strict-Transport-Security", "max-age=31536000")
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

func getFiles(dir string) []string {
	files, err := AssetDir(dir)
	if err != nil {
		return []string{dir}
	}

	var outFiles []string
	for _, filename := range files {
		outFiles = append(outFiles, getFiles(path.Join(dir, filename))...)
	}
	return outFiles
}

func main() {
	log.Println("Starting sadbox.org")

	for _, filename := range getFiles("views") {
		template.Must(templates.Parse(string(MustAsset(filename))))
	}

	// static files
	for _, filename := range getFiles("static") {
		serveStatic(filename)
	}

	geekhack, err := NewGeekhack()
	if err != nil {
		log.Fatal(err)
	}
	defer geekhack.db.Close()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		ctx := NewContext(r)
		if err := templates.ExecuteTemplate(w, "main", ctx); err != nil {
			log.Println(err)
		}
	})

	// Geekhack stats! the geekhack struct will handle the routing to sub-things
	http.Handle("/geekhack/", geekhack)
	// Redirects to the right URL so I don't break old links
	http.Handle("/ghstats", http.RedirectHandler("/geekhack/", http.StatusMovedPermanently))
	http.Handle("/geekhack", http.RedirectHandler("/geekhack/", http.StatusMovedPermanently))

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

	go func() {
		log.Fatal(http.ListenAndServe(":http", RedirectToHTTPS(servemux)))
	}()

	m := autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Cache:      autocert.DirCache("/home/sadbox-web/cert-cache"),
		HostPolicy: autocert.HostWhitelist(hostname_whitelist...),
		Email:      "blue6249@gmail.com",
		ForceRSA:   true,
	}

	tlsconfig := &tls.Config{
		PreferServerCipherSuites: true,
		GetCertificate:           m.GetCertificate,
	}

	server := &http.Server{Addr: ":https", Handler: servemux, TLSConfig: tlsconfig}
	log.Fatal(server.ListenAndServeTLS("", ""))
}
