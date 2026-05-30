package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// memRepo is an in-memory UserStore for handler tests.
type memRepo struct {
	byEmail map[string]struct {
		u    User
		hash string
	}
	nextID int64
}

func newMemRepo() *memRepo {
	return &memRepo{byEmail: map[string]struct {
		u    User
		hash string
	}{}}
}

func (m *memRepo) Create(_ context.Context, email, hash, name string) (User, error) {
	if _, ok := m.byEmail[email]; ok {
		return User{}, ErrEmailTaken
	}
	m.nextID++
	u := User{ID: m.nextID, Email: email, Name: name, Role: "developer"}
	m.byEmail[email] = struct {
		u    User
		hash string
	}{u, hash}
	return u, nil
}

func (m *memRepo) GetByEmail(_ context.Context, email string) (User, string, error) {
	v, ok := m.byEmail[email]
	if !ok {
		return User{}, "", errors.New("not found")
	}
	return v.u, v.hash, nil
}

func newTestHandler() *Handler {
	return NewHandler(newMemRepo(), NewTokenizer("test-secret"))
}

func TestRegisterThenLogin(t *testing.T) {
	h := newTestHandler()

	reg := httptest.NewRequest(http.MethodPost, "/api/auth/register",
		strings.NewReader(`{"email":"a@b.c","password":"pw123456","name":"A"}`))
	regRec := httptest.NewRecorder()
	h.ServeHTTP(regRec, reg)
	if regRec.Code != http.StatusCreated {
		t.Fatalf("register status = %d, want 201; body=%s", regRec.Code, regRec.Body)
	}

	login := httptest.NewRequest(http.MethodPost, "/api/auth/login",
		strings.NewReader(`{"email":"a@b.c","password":"pw123456"}`))
	loginRec := httptest.NewRecorder()
	h.ServeHTTP(loginRec, login)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status = %d, want 200", loginRec.Code)
	}
	if !strings.Contains(loginRec.Body.String(), `"token"`) {
		t.Fatalf("login response missing token: %s", loginRec.Body)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	h := newTestHandler()
	reg := httptest.NewRequest(http.MethodPost, "/api/auth/register",
		strings.NewReader(`{"email":"a@b.c","password":"pw123456","name":"A"}`))
	h.ServeHTTP(httptest.NewRecorder(), reg)

	login := httptest.NewRequest(http.MethodPost, "/api/auth/login",
		strings.NewReader(`{"email":"a@b.c","password":"WRONG"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, login)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestRegisterDuplicateEmailConflicts(t *testing.T) {
	h := newTestHandler()
	body := `{"email":"dup@b.c","password":"pw123456","name":"A"}`
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(body)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(body)))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}

func TestRegisterShortPasswordRejected(t *testing.T) {
	h := newTestHandler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/auth/register",
		strings.NewReader(`{"email":"a@b.c","password":"short","name":"A"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// brokenRepo always fails Create with a generic (non-duplicate) error.
type brokenRepo struct{}

func (b *brokenRepo) Create(_ context.Context, _, _, _ string) (User, error) {
	return User{}, errors.New("db down")
}

func (b *brokenRepo) GetByEmail(_ context.Context, _ string) (User, string, error) {
	return User{}, "", errors.New("db down")
}

func TestRegisterDBErrorReturns500(t *testing.T) {
	h := NewHandler(&brokenRepo{}, NewTokenizer("test-secret"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/auth/register",
		strings.NewReader(`{"email":"a@b.c","password":"pw123456","name":"A"}`)))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}
