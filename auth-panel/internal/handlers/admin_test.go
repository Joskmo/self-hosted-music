package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

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
