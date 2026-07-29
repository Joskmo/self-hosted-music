package handlers

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRegisterPageHandlerShowsUsedInviteAsInvalid(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	originalWebFS := webFS
	webFS = fstest.MapFS{
		"register.html":       &fstest.MapFile{Data: []byte(`<main>{{InviteCode}}<form id="form"></form></main>`)},
		"invite-invalid.html": &fstest.MapFile{Data: []byte(`<main>Ссылка-приглашение недействительна или уже использована</main>`)},
	}
	t.Cleanup(func() { webFS = originalWebFS })

	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM invites WHERE code = \$1 AND used = FALSE\)`).
		WithArgs("already-used").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	req := httptest.NewRequest(http.MethodGet, "/register?invite=already-used", nil)
	rr := httptest.NewRecorder()
	RegisterPageHandler(database)(rr, req)

	if rr.Code != http.StatusGone {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusGone)
	}
	if !strings.Contains(rr.Body.String(), "Ссылка-приглашение недействительна или уже использована") {
		t.Fatalf("response must explain why registration is unavailable: %q", rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), `<form id="form">`) {
		t.Fatalf("used invite must not render a registration form: %q", rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

var _ *sql.DB
