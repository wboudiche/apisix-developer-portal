package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
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

// TestLoginAbsentUserStillReturns401 verifies that an unknown email returns 401
// (not 404 or any other code that would reveal account existence).
func TestLoginAbsentUserStillReturns401(t *testing.T) {
	h := newTestHandler() // empty store — no users registered
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/auth/login",
		strings.NewReader(`{"email":"nobody@example.com","password":"irrelevant"}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("absent user: status = %d, want 401", rec.Code)
	}
}

// TestDummyHashIsGenuineCost12 pins that dummyHash is a real bcrypt hash at
// cost 12.  If it were a fake string, CompareHashAndPassword would return a
// format error instantly, defeating the timing equalization.
func TestDummyHashIsGenuineCost12(t *testing.T) {
	cost, err := bcrypt.Cost([]byte(dummyHash))
	if err != nil {
		t.Fatalf("bcrypt.Cost failed: %v — dummyHash is not a valid bcrypt hash", err)
	}
	if cost != 12 {
		t.Fatalf("dummyHash cost = %d, want 12", cost)
	}
	err = bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte("not-the-password"))
	if !errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		t.Fatalf("expected ErrMismatchedHashAndPassword, got %v", err)
	}
}

// TestRegisterPasswordTooLongReturns400 verifies that a password exceeding
// bcrypt's 72-byte limit is rejected with 400, not 500.
func TestRegisterPasswordTooLongReturns400(t *testing.T) {
	h := newTestHandler()
	longPw := strings.Repeat("a", 73)
	body := `{"email":"a@b.c","password":"` + longPw + `","name":"A"}`
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/auth/register",
		strings.NewReader(body)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("73-byte password: status = %d, want 400; body=%s", rec.Code, rec.Body)
	}
}
