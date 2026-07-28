package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"auth-panel/internal/session"
)

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
