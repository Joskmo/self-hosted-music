package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"auth-panel/internal/metadata"
	"auth-panel/internal/session"
)

type metadataRecord struct {
	ID              string    `json:"id"`
	FilePath        string    `json:"file_path"`
	Source          string    `json:"source"`
	OriginalTitle   string    `json:"original_title"`
	OriginalArtist  string    `json:"original_artist"`
	OriginalAlbum   string    `json:"original_album"`
	SuggestedTitle  string    `json:"suggested_title"`
	SuggestedArtist string    `json:"suggested_artist"`
	SuggestedAlbum  string    `json:"suggested_album"`
	MusicBrainzID   string    `json:"musicbrainz_id"`
	Confidence      float64   `json:"confidence"`
	Status          string    `json:"status"`
	LastError       string    `json:"last_error"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func AdminMetadataListHandler(database *sql.DB, sessions *session.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !RequireAdminSession(w, r, database, sessions) {
			return
		}
		rows, err := database.Query(`SELECT id, file_path, source, original_title, original_artist, original_album, suggested_title, suggested_artist, suggested_album, musicbrainz_id, confidence, status, last_error, updated_at FROM music_metadata ORDER BY updated_at DESC LIMIT 500`)
		if err != nil {
			JSONError(w, "metadata query failed", 500)
			return
		}
		defer rows.Close()
		out := []metadataRecord{}
		for rows.Next() {
			var item metadataRecord
			if err := rows.Scan(&item.ID, &item.FilePath, &item.Source, &item.OriginalTitle, &item.OriginalArtist, &item.OriginalAlbum, &item.SuggestedTitle, &item.SuggestedArtist, &item.SuggestedAlbum, &item.MusicBrainzID, &item.Confidence, &item.Status, &item.LastError, &item.UpdatedAt); err != nil {
				JSONError(w, "metadata scan failed", 500)
				return
			}
			out = append(out, item)
		}
		json.NewEncoder(w).Encode(out)
	}
}

func AdminMetadataUpdateHandler(database *sql.DB, sessions *session.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !RequireAdminSession(w, r, database, sessions) {
			return
		}
		var req struct {
			Title  string `json:"title"`
			Artist string `json:"artist"`
			Album  string `json:"album"`
			Status string `json:"status"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req); err != nil {
			JSONError(w, "invalid request", 400)
			return
		}
		if req.Status != "review" && req.Status != "manual" && req.Status != "resolved" {
			JSONError(w, "invalid status", 400)
			return
		}
		result, err := database.Exec(`UPDATE music_metadata SET suggested_title=$1, suggested_artist=$2, suggested_album=$3, status=$4, updated_at=NOW() WHERE id=$5`, req.Title, req.Artist, req.Album, req.Status, r.PathValue("id"))
		if err != nil {
			JSONError(w, "metadata update failed", 500)
			return
		}
		n, _ := result.RowsAffected()
		if n == 0 {
			JSONError(w, "not found", 404)
			return
		}
		JSONOK(w)
	}
}

func AdminMetadataReprocessHandler(database *sql.DB, sessions *session.Store, processor *metadata.Processor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !RequireAdminSession(w, r, database, sessions) {
			return
		}
		var path string
		if err := database.QueryRow("SELECT file_path FROM music_metadata WHERE id=$1", r.PathValue("id")).Scan(&path); err != nil {
			JSONError(w, "not found", 404)
			return
		}
		if err := processor.Reprocess(context.Background(), path); err != nil {
			JSONError(w, "metadata processing failed", 500)
			return
		}
		JSONOK(w)
	}
}

// AdminMetadataApplyHandler writes metadata only after an explicit admin action.
func AdminMetadataApplyHandler(database *sql.DB, sessions *session.Store, processor *metadata.Processor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !RequireAdminSession(w, r, database, sessions) {
			return
		}
		var path string
		var track metadata.Track
		err := database.QueryRow(`SELECT file_path, suggested_title, suggested_artist, suggested_album FROM music_metadata WHERE id=$1`, r.PathValue("id")).Scan(&path, &track.Title, &track.Artist, &track.Album)
		if err != nil {
			JSONError(w, "not found", http.StatusNotFound)
			return
		}
		if err := processor.Apply(r.Context(), path, track); err != nil {
			JSONError(w, "metadata write failed", http.StatusInternalServerError)
			return
		}
		if err := markMetadataApplied(r.Context(), database, r.PathValue("id")); err != nil {
			JSONError(w, "metadata update failed", http.StatusInternalServerError)
			return
		}
		JSONOK(w)
	}
}

// markMetadataApplied records completion without overwriting the tags captured
// before review. The original values are an audit trail even after a deliberate
// file rewrite by an administrator.
func markMetadataApplied(ctx context.Context, database *sql.DB, id string) error {
	_, err := database.ExecContext(ctx, `UPDATE music_metadata SET status='resolved', last_error='', updated_at=NOW() WHERE id=$1`, id)
	return err
}
