package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"auth-panel/internal/auth"
	"auth-panel/internal/session"
)

func LoginPageHandler(w http.ResponseWriter, r *http.Request) {
	RenderTemplate(w, "login.html", nil)
}

func LoginHandler(database *sql.DB, sessions *session.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			JSONError(w, "Invalid request", http.StatusBadRequest)
			return
		}
		if req.Username == "" || req.Password == "" {
			JSONError(w, "Заполните все поля", http.StatusBadRequest)
			return
		}

		var exists bool
		err := database.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE username = $1)", req.Username).Scan(&exists)
		if err != nil || !exists {
			JSONError(w, "Неверные учётные данные", http.StatusUnauthorized)
			return
		}

		if err := auth.CheckNavidromeCredentials(req.Username, req.Password); err != nil {
			JSONError(w, "Неверные учётные данные", http.StatusUnauthorized)
			return
		}

		token := sessions.Create(req.Username)
		http.SetCookie(w, &http.Cookie{
			Name:     "session",
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   86400,
		})
		JSONOK(w)
	}
}

func LogoutHandler(sessions *session.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session")
		if err == nil {
			sessions.Delete(cookie.Value)
		}
		http.SetCookie(w, &http.Cookie{
			Name:     "session",
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			MaxAge:   -1,
		})
		JSONOK(w)
	}
}

func SessionCheckHandler(sessions *session.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session")
		if err == nil {
			if username, ok := sessions.Valid(cookie.Value); ok {
				json.NewEncoder(w).Encode(map[string]string{"username": username})
				return
			}
		}
		JSONError(w, "Not authenticated", http.StatusUnauthorized)
	}
}

func SessionMeHandler(database *sql.DB, sessions *session.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session")
		if err != nil {
			JSONError(w, "Not authenticated", http.StatusUnauthorized)
			return
		}
		username, ok := sessions.Valid(cookie.Value)
		if !ok {
			JSONError(w, "Not authenticated", http.StatusUnauthorized)
			return
		}
		var isAdmin bool
		err = database.QueryRow("SELECT is_admin FROM users WHERE username = $1", username).Scan(&isAdmin)
		if err != nil {
			JSONError(w, "User not found", http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"username": username, "is_admin": isAdmin})
	}
}

func RegisterPageHandler(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("invite")
	RenderTemplate(w, "register.html", map[string]any{"InviteCode": code})
}

func RegisterHandler(database *sql.DB, sessions *session.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Invite   string `json:"invite"`
			Username string `json:"username"`
			Password string `json:"password"`
			Name     string `json:"name"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			JSONError(w, "Invalid request", http.StatusBadRequest)
			return
		}

		if req.Invite == "" || req.Username == "" || req.Password == "" {
			JSONError(w, "Заполните все обязательные поля", http.StatusBadRequest)
			return
		}

		var inviteID string
		err := database.QueryRow("SELECT id FROM invites WHERE code = $1 AND used = FALSE", req.Invite).Scan(&inviteID)
		if err != nil {
			JSONError(w, "Недействительная или использованная ссылка", http.StatusBadRequest)
			return
		}

		navID, err := auth.CreateNavidromeUser(req.Username, req.Password, req.Name)
		if err != nil {
			JSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		name := req.Name
		if name == "" {
			name = req.Username
		}
		_, err = database.Exec("INSERT INTO users (username, name, navidrome_id) VALUES ($1, $2, $3)",
			req.Username, name, navID)
		if err != nil {
			JSONError(w, "Пользователь уже существует", http.StatusConflict)
			return
		}

		if _, err := database.Exec("UPDATE invites SET used = TRUE WHERE id = $1", inviteID); err != nil {
			// non-fatal
		}

		token := sessions.Create(req.Username)
		http.SetCookie(w, &http.Cookie{
			Name:     "session",
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   86400,
		})

		JSONOK(w)
	}
}

func UploadAuthHandler(sessions *session.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			JSONError(w, "Invalid request", http.StatusBadRequest)
			return
		}
		if req.Username == "" || req.Password == "" {
			JSONError(w, "Заполните все поля", http.StatusBadRequest)
			return
		}
		if err := auth.CheckNavidromeCredentials(req.Username, req.Password); err != nil {
			JSONError(w, "Неверные учётные данные", http.StatusUnauthorized)
			return
		}
		token := sessions.Create(req.Username)
		http.SetCookie(w, &http.Cookie{
			Name:     "session",
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   86400,
		})
		JSONOK(w)
	}
}
