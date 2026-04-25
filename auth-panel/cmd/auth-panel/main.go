package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"auth-panel/internal/auth"
	"auth-panel/internal/db"
	"auth-panel/internal/upload"
)

//go:embed web/*
var webFiles embed.FS

var metubeProxy *httputil.ReverseProxy
var metubeTarget *url.URL

type sessionStore struct {
	mu       sync.Mutex
	sessions map[string]sessionData
}

type sessionData struct {
	username string
	expires  time.Time
}

var sessions = &sessionStore{sessions: make(map[string]sessionData)}

func (s *sessionStore) create(username string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := make([]byte, 32)
	rand.Read(b)
	token := hex.EncodeToString(b)
	s.sessions[token] = sessionData{username: username, expires: time.Now().Add(24 * time.Hour)}
	return token
}

func (s *sessionStore) valid(token string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, ok := s.sessions[token]
	if !ok || time.Now().After(data.expires) {
		delete(s.sessions, token)
		return "", false
	}
	return data.username, true
}

func (s *sessionStore) validFromCookie(r *http.Request) (string, bool) {
	cookie, err := r.Cookie("session")
	if err != nil {
		return "", false
	}
	return s.valid(cookie.Value)
}

func (s *sessionStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for t, exp := range s.sessions {
		if now.After(exp.expires) {
			delete(s.sessions, t)
		}
	}
}

func main() {
	ctx := context.Background()

	database, err := db.New(ctx)
	if err != nil {
		log.Fatalf("db init: %v", err)
	}
	defer database.Close()

	adminUser := os.Getenv("NAVIDROME_ADMIN_USER")
	if adminUser == "" {
		adminUser = "admin"
	}
	adminPass := os.Getenv("NAVIDROME_ADMIN_PASSWORD")

	if err := ensureAdmin(database, adminUser, adminPass); err != nil {
		log.Printf("ensure admin: %v", err)
	}

	metubeURL := os.Getenv("METUBE_URL")
	if metubeURL == "" {
		metubeURL = "http://metube:8081"
	}
	metubeTarget, _ = url.Parse(metubeURL)
	metubeProxy = httputil.NewSingleHostReverseProxy(metubeTarget)
	metubeProxy.ModifyResponse = func(r *http.Response) error {
		if r.Header.Get("Set-Cookie") != "" {
			r.Header.Set("Set-Cookie", strings.ReplaceAll(r.Header.Get("Set-Cookie"), "Path=/", "Path=/metube/proxy/"))
		}
		return nil
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/", indexHandler)
	mux.HandleFunc("GET /login", loginPageHandler)
	mux.HandleFunc("POST /api/login", loginHandler(database))
	mux.HandleFunc("GET /register", registerPageHandler)
	mux.HandleFunc("POST /api/register", registerHandler(database))
	mux.HandleFunc("GET /admin", adminPageHandler)
	mux.HandleFunc("GET /api/admin/invites", adminInvitesHandler(database))
	mux.HandleFunc("POST /api/admin/invites", adminCreateInviteHandler(database))
	mux.HandleFunc("GET /upload", uploadPageHandler)
	mux.HandleFunc("POST /api/upload/auth", uploadAuthHandler)
	mux.HandleFunc("POST /api/upload", uploadHandler)
	mux.HandleFunc("POST /api/upload/zip", uploadZipHandler)
	mux.HandleFunc("/metube/", metubeRouter)
	mux.HandleFunc("GET /metube", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/metube/", http.StatusFound)
	})
	mux.HandleFunc("POST /api/logout", logoutHandler)
	mux.HandleFunc("GET /api/session", sessionCheckHandler)
	mux.HandleFunc("GET /api/me", sessionMeHandler(database))

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	log.Printf("auth-panel listening on :%s", port)
	go func() {
		for range time.Tick(10 * time.Minute) {
			sessions.cleanup()
		}
	}()
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func ensureAdmin(database *sql.DB, username, password string) error {
	var exists bool
	err := database.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE username = $1 AND is_admin = TRUE)", username).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check admin exists: %w", err)
	}
	if exists {
		return nil
	}

	_, err = database.Exec("INSERT INTO users (username, name, is_admin) VALUES ($1, $2, TRUE)",
		username, "Admin")
	if err != nil {
		return fmt.Errorf("insert admin user in local DB: %w", err)
	}

	log.Printf("Local admin user %q created", username)
	return nil
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	cookie, err := r.Cookie("session")
	if err == nil {
		if _, ok := sessions.valid(cookie.Value); ok {
			http.Redirect(w, r, "/upload", http.StatusFound)
			return
		}
	}
	http.Redirect(w, r, "/login", http.StatusFound)
}

func loginPageHandler(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, "login.html", nil)
}

func loginHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "Invalid request", http.StatusBadRequest)
			return
		}
		if req.Username == "" || req.Password == "" {
			jsonError(w, "Заполните все поля", http.StatusBadRequest)
			return
		}

		var exists bool
		err := database.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE username = $1)", req.Username).Scan(&exists)
		if err != nil || !exists {
			jsonError(w, "Неверные учётные данные", http.StatusUnauthorized)
			return
		}

		if err := auth.CheckNavidromeCredentials(req.Username, req.Password); err != nil {
			jsonError(w, "Неверные учётные данные", http.StatusUnauthorized)
			return
		}

		token := sessions.create(req.Username)
		http.SetCookie(w, &http.Cookie{
			Name:     "session",
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   86400,
		})
		jsonOK(w)
	}
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err == nil {
		sessions.mu.Lock()
		delete(sessions.sessions, cookie.Value)
		sessions.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	jsonOK(w)
}

func sessionCheckHandler(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err == nil {
		if username, ok := sessions.valid(cookie.Value); ok {
			json.NewEncoder(w).Encode(map[string]string{"username": username})
			return
		}
	}
	jsonError(w, "Not authenticated", http.StatusUnauthorized)
}

func sessionMeHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session")
		if err != nil {
			jsonError(w, "Not authenticated", http.StatusUnauthorized)
			return
		}
		username, ok := sessions.valid(cookie.Value)
		if !ok {
			jsonError(w, "Not authenticated", http.StatusUnauthorized)
			return
		}
		var isAdmin bool
		err = database.QueryRow("SELECT is_admin FROM users WHERE username = $1", username).Scan(&isAdmin)
		if err != nil {
			jsonError(w, "User not found", http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"username": username, "is_admin": isAdmin})
	}
}

func registerPageHandler(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("invite")
	renderTemplate(w, "register.html", map[string]any{"InviteCode": code})
}

func registerHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Invite   string `json:"invite"`
			Username string `json:"username"`
			Password string `json:"password"`
			Name     string `json:"name"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "Invalid request", http.StatusBadRequest)
			return
		}

		if req.Invite == "" || req.Username == "" || req.Password == "" {
			jsonError(w, "Заполните все обязательные поля", http.StatusBadRequest)
			return
		}

		var inviteID string
		err := database.QueryRow("SELECT id FROM invites WHERE code = $1 AND used = FALSE", req.Invite).Scan(&inviteID)
		if err != nil {
			jsonError(w, "Недействительная или использованная ссылка", http.StatusBadRequest)
			return
		}

		if err := auth.CreateNavidromeUser(req.Username, req.Password, req.Name); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		name := req.Name
		if name == "" {
			name = req.Username
		}
		_, err = database.Exec("INSERT INTO users (username, name) VALUES ($1, $2)",
			req.Username, name)
		if err != nil {
			jsonError(w, "Пользователь уже существует", http.StatusConflict)
			return
		}

		if _, err := database.Exec("UPDATE invites SET used = TRUE WHERE id = $1", inviteID); err != nil {
			log.Printf("mark invite used: %v", err)
		}

		token := sessions.create(req.Username)
		http.SetCookie(w, &http.Cookie{
			Name:     "session",
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   86400,
		})

		jsonOK(w)
	}
}

func adminPageHandler(w http.ResponseWriter, r *http.Request) {
	username := requireSession(w, r)
	if username == "" {
		return
	}
	renderTemplate(w, "admin.html", nil)
}

func adminInvitesHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAdminSession(w, r, database) {
			return
		}

		rows, err := database.Query("SELECT code, used, created_at FROM invites ORDER BY created_at DESC")
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var invites []map[string]any
		for rows.Next() {
			var code string
			var used bool
			var createdAt string
			rows.Scan(&code, &used, &createdAt)
			invites = append(invites, map[string]any{
				"code":       code,
				"used":       used,
				"created_at": createdAt,
			})
		}

		if invites == nil {
			invites = []map[string]any{}
		}

		json.NewEncoder(w).Encode(invites)
	}
}

func adminCreateInviteHandler(database *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireAdminSession(w, r, database) {
			return
		}

		code := auth.GenerateInviteCode()
		if _, err := database.Exec("INSERT INTO invites (code) VALUES ($1)", code); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string]string{"code": code})
	}
}

func requireSession(w http.ResponseWriter, r *http.Request) string {
	cookie, err := r.Cookie("session")
	if err == nil {
		if username, ok := sessions.valid(cookie.Value); ok {
			return username
		}
	}
	http.Redirect(w, r, "/login", http.StatusFound)
	return ""
}

func requireAdminSession(w http.ResponseWriter, r *http.Request, database *sql.DB) bool {
	cookie, err := r.Cookie("session")
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return false
	}
	username, ok := sessions.valid(cookie.Value)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return false
	}

	var isAdmin bool
	err = database.QueryRow("SELECT is_admin FROM users WHERE username = $1", username).Scan(&isAdmin)
	if err != nil || !isAdmin {
		jsonError(w, "Доступ запрещён", http.StatusForbidden)
		return false
	}
	return true
}

func uploadPageHandler(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, "upload.html", nil)
}

func uploadAuthHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request", http.StatusBadRequest)
		return
	}
	if req.Username == "" || req.Password == "" {
		jsonError(w, "Заполните все поля", http.StatusBadRequest)
		return
	}
	if err := auth.CheckNavidromeCredentials(req.Username, req.Password); err != nil {
		jsonError(w, "Неверные учётные данные", http.StatusUnauthorized)
		return
	}
	token := sessions.create(req.Username)
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	})
	jsonOK(w)
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if !requireNavidromeAuth(w, r) {
		return
	}

	musicDir := os.Getenv("MUSIC_DIR")
	if musicDir == "" {
		musicDir = "/music"
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		jsonError(w, "Parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	files := r.MultipartForm.File["files"]
	saved, err := upload.SaveUploads(musicDir, files)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]any{"ok": true, "saved": saved})
}

func uploadZipHandler(w http.ResponseWriter, r *http.Request) {
	if !requireNavidromeAuth(w, r) {
		return
	}

	musicDir := os.Getenv("MUSIC_DIR")
	if musicDir == "" {
		musicDir = "/music"
	}

	if err := r.ParseMultipartForm(128 << 20); err != nil {
		jsonError(w, "Parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	fh := r.MultipartForm.File["file"]
	if len(fh) == 0 {
		jsonError(w, "No file", http.StatusBadRequest)
		return
	}

	saved, err := upload.SaveUploadedZip(musicDir, fh[0])
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]any{"ok": true, "saved": saved})
}

func requireNavidromeAuth(w http.ResponseWriter, r *http.Request) bool {
	cookie, err := r.Cookie("session")
	if err == nil {
		if _, ok := sessions.valid(cookie.Value); ok {
			return true
		}
	}
	username := r.Header.Get("X-Navidrome-User")
	password := r.Header.Get("X-Navidrome-Pass")

	if username == "" || password == "" {
		jsonError(w, "Требуется авторизация", http.StatusUnauthorized)
		return false
	}

	if err := auth.CheckNavidromeCredentials(username, password); err != nil {
		jsonError(w, "Неверные учётные данные", http.StatusUnauthorized)
		return false
	}

	return true
}

func metubePageHandler(w http.ResponseWriter, r *http.Request) {
	_, ok := sessions.validFromCookie(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	renderTemplate(w, "metube.html", nil)
}

func metubeRouter(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "/metube" || path == "/metube/" || path == "/metube/login" {
		metubePageHandler(w, r)
		return
	}
	if path == "/metube/view" {
		metubePageHandler(w, r)
		return
	}
	if strings.HasPrefix(path, "/metube/proxy/") || strings.HasPrefix(path, "/metube/proxy") {
		_, ok := sessions.validFromCookie(r)
		if !ok {
			jsonError(w, "Требуется авторизация", http.StatusUnauthorized)
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

		metubeProxy.ServeHTTP(w, r)
		return
	}
	http.NotFound(w, r)
}

func jsonOK(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func jsonError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func renderTemplate(w http.ResponseWriter, name string, data map[string]any) {
	webSub, _ := fs.Sub(webFiles, "web")
	content, err := fs.ReadFile(webSub, name)
	if err != nil {
		http.Error(w, "Template not found", http.StatusNotFound)
		return
	}

	html := string(content)
	if data != nil {
		for key, val := range data {
			html = strings.ReplaceAll(html, "{{"+key+"}}", fmt.Sprint(val))
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

func proxyWebSocket(w http.ResponseWriter, r *http.Request) {
	target := *metubeTarget
	target.Path = r.URL.Path
	target.RawQuery = r.URL.RawQuery

	connBackend, _, err := websocket.DefaultDialer.Dial(target.String(), nil)
	if err != nil {
		log.Printf("WebSocket dial error: %v", err)
		http.Error(w, "WebSocket error", http.StatusBadGateway)
		return
	}
	defer connBackend.Close()

	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool { return true },
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
