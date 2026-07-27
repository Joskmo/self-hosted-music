package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"

	"auth-panel/internal/auth"
	"auth-panel/internal/session"
)

func AdminPageHandler(w http.ResponseWriter, r *http.Request, sessions *session.Store) {
	if RequireSession(w, r, sessions) == "" {
		return
	}
	RenderTemplate(w, "admin.html", map[string]any{"NavidromeURL": PublicNavidromeURL()})
}

func AdminInvitesHandler(database *sql.DB, sessions *session.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !RequireAdminSession(w, r, database, sessions) {
			return
		}

		rows, err := database.Query("SELECT code, used, created_at FROM invites ORDER BY created_at DESC")
		if err != nil {
			JSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var invites []map[string]any
		for rows.Next() {
			var code string
			var used bool
			var createdAt string
			if err := rows.Scan(&code, &used, &createdAt); err != nil {
				continue
			}
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

func AdminCreateInviteHandler(database *sql.DB, sessions *session.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !RequireAdminSession(w, r, database, sessions) {
			return
		}

		code := auth.GenerateInviteCode()
		if _, err := database.Exec("INSERT INTO invites (code) VALUES ($1)", code); err != nil {
			JSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string]string{"code": code})
	}
}

func AdminUsersHandler(database *sql.DB, sessions *session.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !RequireAdminSession(w, r, database, sessions) {
			return
		}

		token, err := auth.GetNavidromeAdminToken()
		if err != nil {
			JSONError(w, "Navidrome admin auth failed: "+err.Error(), http.StatusInternalServerError)
			return
		}

		users, err := auth.GetNavidromeUsers(token)
		if err != nil {
			JSONError(w, "Failed to fetch Navidrome users: "+err.Error(), http.StatusInternalServerError)
			return
		}

		for _, u := range users {
			_, err := database.Exec(
				`INSERT INTO users (username, name, is_admin, navidrome_id)
				 VALUES ($1, $2, $3, $4)
				 ON CONFLICT (username) DO UPDATE
				 SET name = EXCLUDED.name, is_admin = EXCLUDED.is_admin, navidrome_id = EXCLUDED.navidrome_id`,
				u.UserName, u.Name, u.IsAdmin, u.ID,
			)
			if err != nil {
				log.Printf("sync user %s: %v", u.UserName, err)
			}
		}

		rows, err := database.Query("SELECT id, username, name, is_admin, navidrome_id, created_at FROM users ORDER BY created_at DESC")
		if err != nil {
			JSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		mainAdmin := MainAdminUser()

		var result []map[string]any
		for rows.Next() {
			var id, username, name, navID, createdAt string
			var isAdmin bool
			if err := rows.Scan(&id, &username, &name, &isAdmin, &navID, &createdAt); err != nil {
				continue
			}
			result = append(result, map[string]any{
				"id":            id,
				"username":      username,
				"name":          name,
				"is_admin":      isAdmin,
				"is_main_admin": username == mainAdmin,
				"navidrome_id":  navID,
				"created_at":    createdAt,
			})
		}
		if result == nil {
			result = []map[string]any{}
		}
		json.NewEncoder(w).Encode(result)
	}
}

func AdminUpdateRoleHandler(database *sql.DB, sessions *session.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !RequireAdminSession(w, r, database, sessions) {
			return
		}

		cookie, _ := r.Cookie("session")
		currentAdmin, _ := sessions.Valid(cookie.Value)

		userID := r.PathValue("id")
		var req struct {
			IsAdmin bool `json:"is_admin"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			JSONError(w, "Invalid request", http.StatusBadRequest)
			return
		}

		var targetUsername string
		if err := database.QueryRow("SELECT username FROM users WHERE id = $1", userID).Scan(&targetUsername); err != nil {
			JSONError(w, "User not found", http.StatusNotFound)
			return
		}

		mainAdmin := MainAdminUser()

		if targetUsername == mainAdmin {
			JSONError(w, "Нельзя изменить роль главного администратора", http.StatusForbidden)
			return
		}
		if targetUsername == currentAdmin {
			JSONError(w, "Нельзя изменить свою роль", http.StatusForbidden)
			return
		}

		// prevent removing last admin
		if !req.IsAdmin {
			var adminCount int
			if err := database.QueryRow("SELECT COUNT(*) FROM users WHERE is_admin = TRUE").Scan(&adminCount); err == nil && adminCount <= 1 {
				var targetIsAdmin bool
				database.QueryRow("SELECT is_admin FROM users WHERE id = $1", userID).Scan(&targetIsAdmin)
				if targetIsAdmin {
					JSONError(w, "Нельзя снять роль с последнего администратора", http.StatusForbidden)
					return
				}
			}
		}

		var navID string
		if err := database.QueryRow("SELECT navidrome_id FROM users WHERE id = $1", userID).Scan(&navID); err != nil || navID == "" {
			JSONError(w, "Пользователь не синхронизирован с Navidrome", http.StatusInternalServerError)
			return
		}

		token, err := auth.GetNavidromeAdminToken()
		if err != nil {
			JSONError(w, "Navidrome admin auth failed: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if err := auth.UpdateNavidromeUser(token, navID, req.IsAdmin); err != nil {
			JSONError(w, "Navidrome update failed: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if _, err := database.Exec("UPDATE users SET is_admin = $1 WHERE id = $2", req.IsAdmin, userID); err != nil {
			JSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		JSONOK(w)
	}
}

func AdminDeleteUserHandler(database *sql.DB, sessions *session.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !RequireAdminSession(w, r, database, sessions) {
			return
		}

		cookie, _ := r.Cookie("session")
		currentAdmin, _ := sessions.Valid(cookie.Value)

		userID := r.PathValue("id")

		var targetUsername string
		if err := database.QueryRow("SELECT username FROM users WHERE id = $1", userID).Scan(&targetUsername); err != nil {
			JSONError(w, "User not found", http.StatusNotFound)
			return
		}

		mainAdmin := MainAdminUser()

		if targetUsername == mainAdmin {
			JSONError(w, "Нельзя удалить главного администратора", http.StatusForbidden)
			return
		}
		if targetUsername == currentAdmin {
			JSONError(w, "Нельзя удалить самого себя", http.StatusForbidden)
			return
		}

		// prevent deleting last admin
		var adminCount int
		if err := database.QueryRow("SELECT COUNT(*) FROM users WHERE is_admin = TRUE").Scan(&adminCount); err == nil && adminCount <= 1 {
			var targetIsAdmin bool
			database.QueryRow("SELECT is_admin FROM users WHERE id = $1", userID).Scan(&targetIsAdmin)
			if targetIsAdmin {
				JSONError(w, "Нельзя удалить последнего администратора", http.StatusForbidden)
				return
			}
		}

		var navID string
		if err := database.QueryRow("SELECT navidrome_id FROM users WHERE id = $1", userID).Scan(&navID); err != nil || navID == "" {
			JSONError(w, "Пользователь не синхронизирован с Navidrome", http.StatusInternalServerError)
			return
		}

		token, err := auth.GetNavidromeAdminToken()
		if err != nil {
			JSONError(w, "Navidrome admin auth failed: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if err := auth.DeleteNavidromeUser(token, navID); err != nil {
			JSONError(w, "Navidrome delete failed: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if _, err := database.Exec("DELETE FROM users WHERE id = $1", userID); err != nil {
			JSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		JSONOK(w)
	}
}
