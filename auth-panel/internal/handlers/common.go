package handlers

import (
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"

	"auth-panel/internal/auth"
	"auth-panel/internal/session"
)

var webFS fs.FS

func InitWebFS(e embed.FS, path string) {
	sub, _ := fs.Sub(e, path)
	webFS = sub
}

func JSONOK(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func JSONError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func RenderTemplate(w http.ResponseWriter, name string, data map[string]any) {
	content, err := fs.ReadFile(webFS, name)
	if err != nil {
		http.Error(w, "Template not found", http.StatusNotFound)
		return
	}

	html := string(content)
	for key, val := range data {
		html = strings.ReplaceAll(html, "{{"+key+"}}", fmt.Sprint(val))
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Write([]byte(html))
}

func PublicNavidromeURL() string {
	if u := os.Getenv("NAVIDROME_PUBLIC_URL"); u != "" {
		return u
	}
	return "http://localhost:4533"
}

func RequireSession(w http.ResponseWriter, r *http.Request, sessions *session.Store) string {
	cookie, err := r.Cookie("session")
	if err == nil {
		if username, ok := sessions.Valid(cookie.Value); ok {
			return username
		}
	}
	http.Redirect(w, r, "/login", http.StatusFound)
	return ""
}

func RequireAdminSession(w http.ResponseWriter, r *http.Request, database *sql.DB, sessions *session.Store) bool {
	cookie, err := r.Cookie("session")
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return false
	}
	username, ok := sessions.Valid(cookie.Value)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return false
	}

	var isAdmin bool
	err = database.QueryRow("SELECT is_admin FROM users WHERE username = $1", username).Scan(&isAdmin)
	if err != nil || !isAdmin {
		JSONError(w, "Доступ запрещён", http.StatusForbidden)
		return false
	}
	return true
}

func RequireNavidromeAuth(w http.ResponseWriter, r *http.Request, sessions *session.Store) bool {
	cookie, err := r.Cookie("session")
	if err == nil {
		if _, ok := sessions.Valid(cookie.Value); ok {
			return true
		}
	}
	username := r.Header.Get("X-Navidrome-User")
	password := r.Header.Get("X-Navidrome-Pass")

	if username == "" || password == "" {
		JSONError(w, "Требуется авторизация", http.StatusUnauthorized)
		return false
	}

	if err := auth.CheckNavidromeCredentials(username, password); err != nil {
		JSONError(w, "Неверные учётные данные", http.StatusUnauthorized)
		return false
	}

	return true
}

func MainAdminUser() string {
	if u := os.Getenv("NAVIDROME_ADMIN_USER"); u != "" {
		return u
	}
	return "admin"
}
