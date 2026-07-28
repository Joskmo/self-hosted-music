package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"auth-panel/internal/metadata"
	"auth-panel/internal/session"
)

func TestMarkMetadataAppliedPreservesOriginalMetadata(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	mock.ExpectExec(`UPDATE music_metadata SET status='resolved', last_error='', updated_at=NOW\(\) WHERE id=\$1`).
		WithArgs("metadata-id").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := markMetadataApplied(context.Background(), database, "metadata-id"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAdminPageHandlerRejectsNonAdminSession(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	sessions := session.New()
	token := sessions.Create("member")
	mock.ExpectQuery("SELECT is_admin FROM users WHERE username = \\$1").
		WithArgs("member").
		WillReturnRows(sqlmock.NewRows([]string{"is_admin"}).AddRow(false))

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rr := httptest.NewRecorder()

	AdminPageHandler(rr, req, database, sessions)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusForbidden)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAdminMetadataScanHandlerStartsScanForAdmin(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	sessions := session.New()
	token := sessions.Create("admin")
	mock.ExpectQuery("SELECT is_admin FROM users WHERE username = \\$1").
		WithArgs("admin").
		WillReturnRows(sqlmock.NewRows([]string{"is_admin"}).AddRow(true))
	started := false
	req := httptest.NewRequest(http.MethodPost, "/api/admin/metadata/scan", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rr := httptest.NewRecorder()

	AdminMetadataScanHandler(database, sessions, func() bool {
		started = true
		return true
	})(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusAccepted)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	if !started {
		t.Fatal("metadata scan was not started")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAdminMetadataScanStatusHandlerReturnsRunningAndLastCheck(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	sessions := session.New()
	token := sessions.Create("admin")
	mock.ExpectQuery("SELECT is_admin FROM users WHERE username = \\$1").
		WithArgs("admin").
		WillReturnRows(sqlmock.NewRows([]string{"is_admin"}).AddRow(true))
	startedAt := time.Date(2026, time.July, 28, 15, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(2 * time.Minute)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/metadata/scan", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rr := httptest.NewRecorder()

	AdminMetadataScanStatusHandler(database, sessions, func() metadata.ScanStatus {
		return metadata.ScanStatus{Running: true, StartedAt: startedAt, LastFinishedAt: finishedAt}
	})(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	var got metadata.ScanStatus
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if !got.Running || !got.StartedAt.Equal(startedAt) || !got.LastFinishedAt.Equal(finishedAt) {
		t.Fatalf("scan status = %#v, want running status with timestamps", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAdminMetadataApplyHandlerRejectsNonAdminSession(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	sessions := session.New()
	token := sessions.Create("member")
	mock.ExpectQuery("SELECT is_admin FROM users WHERE username = \\$1").
		WithArgs("member").
		WillReturnRows(sqlmock.NewRows([]string{"is_admin"}).AddRow(false))
	req := httptest.NewRequest(http.MethodPost, "/api/admin/metadata/id/apply", nil)
	req.SetPathValue("id", "id")
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rr := httptest.NewRecorder()

	AdminMetadataApplyHandler(database, sessions, metadata.NewProcessor(database, t.TempDir()))(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusForbidden)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAdminMetadataApplyHandlerRejectsRecordOutsideReview(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	sessions := session.New()
	token := sessions.Create("admin")
	mock.ExpectQuery("SELECT is_admin FROM users WHERE username = \\$1").
		WithArgs("admin").
		WillReturnRows(sqlmock.NewRows([]string{"is_admin"}).AddRow(true))
	mock.ExpectQuery(`SELECT file_path, suggested_title, suggested_artist, suggested_album, status FROM music_metadata WHERE id=\$1`).
		WithArgs("metadata-id").
		WillReturnRows(sqlmock.NewRows([]string{"file_path", "suggested_title", "suggested_artist", "suggested_album", "status"}).
			AddRow("track.mp3", "Song", "Artist", "Album", "resolved"))
	req := httptest.NewRequest(http.MethodPost, "/api/admin/metadata/metadata-id/apply", nil)
	req.SetPathValue("id", "metadata-id")
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rr := httptest.NewRecorder()

	AdminMetadataApplyHandler(database, sessions, metadata.NewProcessor(database, t.TempDir()))(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusConflict)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
