package main

import (
	"context"
	"database/sql"
	"embed"
	"log"
	"net/http"
	"os"
	"time"

	"auth-panel/internal/db"
	"auth-panel/internal/handlers"
	"auth-panel/internal/metube"
	"auth-panel/internal/session"
)

//go:embed web/*
var webFiles embed.FS

func main() {
	ctx := context.Background()

	database, err := db.New(ctx)
	if err != nil {
		log.Fatalf("db init: %v", err)
	}
	defer database.Close()

	sessions := session.New()
	handlers.InitWebFS(webFiles, "web")
	metube.Init()

	adminUser := os.Getenv("NAVIDROME_ADMIN_USER")
	if adminUser == "" {
		adminUser = "admin"
	}
	if err := ensureAdmin(database, adminUser); err != nil {
		log.Printf("ensure admin: %v", err)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/", indexHandler(sessions))
	mux.HandleFunc("GET /login", handlers.LoginPageHandler)
	mux.HandleFunc("POST /api/login", handlers.LoginHandler(database, sessions))
	mux.HandleFunc("GET /register", handlers.RegisterPageHandler)
	mux.HandleFunc("POST /api/register", handlers.RegisterHandler(database, sessions))
	mux.HandleFunc("GET /admin", adminPageHandler(sessions))
	mux.HandleFunc("GET /api/admin/invites", handlers.AdminInvitesHandler(database, sessions))
	mux.HandleFunc("POST /api/admin/invites", handlers.AdminCreateInviteHandler(database, sessions))
	mux.HandleFunc("GET /api/admin/users", handlers.AdminUsersHandler(database, sessions))
	mux.HandleFunc("PUT /api/admin/users/{id}/role", handlers.AdminUpdateRoleHandler(database, sessions))
	mux.HandleFunc("DELETE /api/admin/users/{id}", handlers.AdminDeleteUserHandler(database, sessions))
	mux.HandleFunc("GET /upload", handlers.UploadPageHandler)
	mux.HandleFunc("POST /api/upload/auth", handlers.UploadAuthHandler(sessions))
	mux.HandleFunc("POST /api/upload", handlers.UploadHandler(sessions))
	mux.HandleFunc("POST /api/upload/zip", handlers.UploadZipHandler(sessions))
	mux.HandleFunc("/metube/", func(w http.ResponseWriter, r *http.Request) {
		metube.Router(w, r, sessions)
	})
	mux.HandleFunc("GET /metube", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/metube/", http.StatusFound)
	})
	mux.HandleFunc("POST /api/logout", handlers.LogoutHandler(sessions))
	mux.HandleFunc("GET /api/session", handlers.SessionCheckHandler(sessions))
	mux.HandleFunc("GET /api/me", handlers.SessionMeHandler(database, sessions))

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	log.Printf("auth-panel listening on :%s", port)
	go func() {
		for range time.Tick(10 * time.Minute) {
			sessions.Cleanup()
		}
	}()
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func ensureAdmin(database *sql.DB, username string) error {
	var exists bool
	err := database.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE username = $1 AND is_admin = TRUE)", username).Scan(&exists)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	_, err = database.Exec("INSERT INTO users (username, name, is_admin) VALUES ($1, $2, TRUE)", username, "Admin")
	if err != nil {
		return err
	}

	log.Printf("Local admin user %q created", username)
	return nil
}

func indexHandler(sessions *session.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		cookie, err := r.Cookie("session")
		if err == nil {
			if _, ok := sessions.Valid(cookie.Value); ok {
				http.Redirect(w, r, "/upload", http.StatusFound)
				return
			}
		}
		http.Redirect(w, r, "/login", http.StatusFound)
	}
}

func adminPageHandler(sessions *session.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handlers.AdminPageHandler(w, r, sessions)
	}
}
