package metube

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"auth-panel/internal/handlers"
	"auth-panel/internal/session"

	"github.com/gorilla/websocket"
)

var (
	proxy  *httputil.ReverseProxy
	target *url.URL
)

func Init() {
	metubeURL := os.Getenv("METUBE_URL")
	if metubeURL == "" {
		metubeURL = "http://metube:8081"
	}
	target, _ = url.Parse(metubeURL)
	proxy = httputil.NewSingleHostReverseProxy(target)
	proxy.ModifyResponse = func(r *http.Response) error {
		if r.Header.Get("Set-Cookie") != "" {
			r.Header.Set("Set-Cookie", strings.ReplaceAll(r.Header.Get("Set-Cookie"), "Path=/", "Path=/metube/proxy/"))
		}
		return nil
	}
}

func PageHandler(w http.ResponseWriter, r *http.Request, sessions *session.Store) {
	_, ok := sessions.ValidFromCookie(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	handlers.RenderTemplate(w, "metube.html", nil)
}

func Router(w http.ResponseWriter, r *http.Request, sessions *session.Store) {
	path := r.URL.Path
	if path == "/metube" || path == "/metube/" || path == "/metube/login" || path == "/metube/view" {
		PageHandler(w, r, sessions)
		return
	}
	if strings.HasPrefix(path, "/metube/proxy/") || strings.HasPrefix(path, "/metube/proxy") {
		_, ok := sessions.ValidFromCookie(r)
		if !ok {
			handlers.JSONError(w, "Требуется авторизация", http.StatusUnauthorized)
			return
		}

		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/metube/proxy")
		if r.URL.Path == "" {
			r.URL.Path = "/"
		}
		q := r.URL.Query()
		q.Del("u")
		q.Del("p")
		r.URL.RawQuery = q.Encode()

		if strings.ToLower(r.Header.Get("Upgrade")) == "websocket" {
			proxyWebSocket(w, r)
			return
		}

		proxy.ServeHTTP(w, r)
		return
	}
	http.NotFound(w, r)
}

func proxyWebSocket(w http.ResponseWriter, r *http.Request) {
	t := *target
	t.Path = r.URL.Path
	t.RawQuery = r.URL.RawQuery

	connBackend, _, err := websocket.DefaultDialer.Dial(t.String(), nil)
	if err != nil {
		log.Printf("WebSocket dial error: %v", err)
		http.Error(w, "WebSocket error", http.StatusBadGateway)
		return
	}
	defer connBackend.Close()

	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     func(r *http.Request) bool { return true },
	}
	connClient, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer connClient.Close()

	errCh := make(chan error, 2)
	go relayWsMessages(connClient, connBackend, errCh)
	go relayWsMessages(connBackend, connClient, errCh)
	<-errCh
}

func relayWsMessages(dst, src *websocket.Conn, errCh chan<- error) {
	for {
		msgType, msg, err := src.ReadMessage()
		if err != nil {
			errCh <- err
			return
		}
		if err := dst.WriteMessage(msgType, msg); err != nil {
			errCh <- err
			return
		}
	}
}
