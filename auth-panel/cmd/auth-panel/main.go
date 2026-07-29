package main

import (
	"context"
	"database/sql"
	"embed"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"auth-panel/internal/db"
	"auth-panel/internal/handlers"
	"auth-panel/internal/metadata"
	"auth-panel/internal/metube"
	"auth-panel/internal/session"
	"auth-panel/internal/soundcloud"
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

	musicDir := os.Getenv("MUSIC_DIR")
	if musicDir == "" {
		musicDir = "/music"
	}
	metadataProcessor := metadata.NewProcessor(database, musicDir)
	metadataProcessor.StartScan()
	go func() {
		for range time.Tick(5 * time.Minute) {
			metadataProcessor.StartScan()
		}
	}()

	sessions := session.New()
	handlers.InitWebFS(webFiles, "web")
	metube.Init()
	soundCloudClient := soundcloud.NewClient("https://soundcloud.com", soundCloudHTTPClient())
	meTubeClient := metube.NewClient(os.Getenv("METUBE_URL"), nil)
	authPanelOrigin := os.Getenv("AUTH_PANEL_ORIGIN")
	if parsed, err := url.Parse(authPanelOrigin); err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		log.Fatal("AUTH_PANEL_ORIGIN must be a canonical HTTPS origin")
	}

	adminUser := os.Getenv("NAVIDROME_ADMIN_USER")
	if adminUser == "" {
		adminUser = "admin"
	}
	if err := ensureAdmin(database, adminUser); err != nil {
		log.Printf("ensure admin: %v", err)
	}

	mux := http.NewServeMux()

	stylePreviewDomain := os.Getenv("STYLE_PREVIEW_DOMAIN")
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && stylePreviewDomain != "" && r.Host == stylePreviewDomain {
			handlers.RenderTemplate(w, "style-preview.html", nil)
			return
		}
		indexHandler(sessions)(w, r)
	})
	mux.HandleFunc("GET /login", handlers.LoginPageHandler)
	mux.HandleFunc("POST /api/login", handlers.LoginHandler(database, sessions))
	mux.HandleFunc("GET /register", handlers.RegisterPageHandler)
	mux.HandleFunc("POST /api/register", handlers.RegisterHandler(database, sessions))
	mux.HandleFunc("GET /admin", adminPageHandler(database, sessions))
	mux.HandleFunc("GET /api/admin/invites", handlers.AdminInvitesHandler(database, sessions))
	mux.HandleFunc("POST /api/admin/invites", handlers.AdminCreateInviteHandler(database, sessions))
	mux.HandleFunc("GET /api/admin/users", handlers.AdminUsersHandler(database, sessions))
	mux.HandleFunc("PUT /api/admin/users/{id}/role", handlers.AdminUpdateRoleHandler(database, sessions))
	mux.HandleFunc("DELETE /api/admin/users/{id}", handlers.AdminDeleteUserHandler(database, sessions))
	mux.HandleFunc("GET /api/admin/metadata", handlers.AdminMetadataListHandler(database, sessions))
	mux.HandleFunc("GET /api/admin/metadata/scan", handlers.AdminMetadataScanStatusHandler(database, sessions, metadataProcessor.Status))
	mux.HandleFunc("POST /api/admin/metadata/scan", handlers.AdminMetadataScanHandler(database, sessions, metadataProcessor.StartScan))
	mux.HandleFunc("PUT /api/admin/metadata/{id}", handlers.AdminMetadataUpdateHandler(database, sessions))
	mux.HandleFunc("POST /api/admin/metadata/{id}/reprocess", handlers.AdminMetadataReprocessHandler(database, sessions, metadataProcessor))
	mux.HandleFunc("POST /api/admin/metadata/{id}/apply", handlers.AdminMetadataApplyHandler(database, sessions, metadataProcessor))
	mux.HandleFunc("GET /upload", handlers.UploadPageHandler)
	mux.HandleFunc("GET /discover", func(w http.ResponseWriter, r *http.Request) {
		handlers.SoundCloudDiscoverPageHandler(w, r, sessions)
	})
	mux.HandleFunc("POST /api/upload/auth", handlers.UploadAuthHandler(sessions))
	mux.HandleFunc("POST /api/upload", handlers.UploadHandler(sessions))
	mux.HandleFunc("POST /api/upload/zip", handlers.UploadZipHandler(sessions))
	mux.HandleFunc("GET /api/discover/soundcloud", handlers.SoundCloudSearchHandler(sessions, soundCloudClient))
	mux.HandleFunc("POST /api/discover/soundcloud/add", handlers.SoundCloudAddHandler(sessions, meTubeClient, authPanelOrigin))
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

func soundCloudHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if proxyRaw := os.Getenv("SOUNDCLOUD_PROXY_URL"); proxyRaw != "" {
		if proxyURL, err := url.Parse(proxyRaw); err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		} else {
			log.Printf("invalid SOUNDCLOUD_PROXY_URL: %v", err)
		}
	}
	return &http.Client{Timeout: 20 * time.Second, Transport: transport}
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

func adminPageHandler(database *sql.DB, sessions *session.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handlers.AdminPageHandler(w, r, database, sessions)
	}
}
