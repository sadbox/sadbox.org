package main

import (
	"crypto/tls"
	"html/template"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"

	"github.com/daaku/go.httpgzip"
	"golang.org/x/crypto/acme/autocert"
)

var templates = template.Must(template.New("").Funcs(template.FuncMap{"add": func(a, b int) int { return a + b }}).ParseGlob("./views/*.tmpl"))

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
	matchedBoards := &Keyboards{make(map[string]string)}
	for _, keyboard := range keyboards {
		dir, file := path.Split(keyboard)
		matchedBoards.Keyboards[path.Join("/", dir, file)] = path.Join("/", dir, "thumbs", file)
	}
	ctx := NewContext(r)
	ctx.keyboards = matchedBoards
	if err := templates.ExecuteTemplate(w, "keyboards.tmpl", ctx); err != nil {
		log.Println(err)
	}
}

type WebsiteName struct {
	Title, Brand string
}

type TemplateContext struct {
	geekhack  *Geekhack
	webname   *WebsiteName
	keyboards *Keyboards
}

func NewContext(r *http.Request) *TemplateContext {
	return &TemplateContext{
		webname: &WebsiteName{
			strings.Replace(r.Host, ".", " &middot; ", -1),
			r.Host,
		},
	}
}

func serveStatic(filename string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=31536000")
		http.ServeFile(w, r, filename)
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

func main() {
	log.Println("Starting sadbox.org")

	geekhack, err := NewGeekhack()
	if err != nil {
		log.Fatal(err)
	}
	defer geekhack.db.Close()

	// These files have to be here
	http.HandleFunc("/favicon.ico", serveStatic("./static/favicon.ico"))
	http.HandleFunc("/sitemap.xml", serveStatic("./static/sitemap.xml"))
	http.HandleFunc("/robots.txt", serveStatic("./static/robots.txt"))
	http.HandleFunc("/humans.txt", serveStatic("./static/humans.txt"))
	http.HandleFunc("/static/jquery.min.js", serveStatic("./vendor/jquery.min.js"))
	http.HandleFunc("/static/highcharts.js", serveStatic("./vendor/highcharts.js"))
	http.HandleFunc("/static/bootstrap.min.css", serveStatic("./vendor/bootstrap.min.css"))
	http.HandleFunc("/mu-fea81392-5746180a-5e50de1d-fb4a7b05.txt", serveStatic("./static/blitz.txt"))

	// The plain-jane stuff I serve up
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		ctx := NewContext(r)
		if err := templates.ExecuteTemplate(w, "main.tmpl", ctx); err != nil {
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
		Prompt: autocert.AcceptTOS,
		Cache:  autocert.DirCache("/home/sadbox-web/cert-cache"),
		HostPolicy: autocert.HostWhitelist(
			"www.sadbox.org", "sadbox.org",
			"www.sadbox.es", "sadbox.es",
			"www.geekwhack.org", "geekwhack.org"),
	}

	tlsconfig := &tls.Config{
		MinVersion:               tls.VersionTLS10, // Disable SSLv3
		PreferServerCipherSuites: true,
		GetCertificate:           m.GetCertificate,
	}

	server := &http.Server{Addr: ":https", Handler: servemux, TLSConfig: tlsconfig}
	log.Fatal(server.ListenAndServeTLS("", ""))
}
