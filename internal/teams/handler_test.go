package teams

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"apisix-portal/internal/auth"
)

type fakeStore struct {
	listed   []TeamSummary
	created  string
	addEmail string
	addErr   error
	roleFn   func(teamID, userID int64) (string, bool, error)
}

func (f *fakeStore) ListForUser(_ context.Context, _ int64) ([]TeamSummary, error) {
	return f.listed, nil
}
func (f *fakeStore) Create(_ context.Context, name string, _ int64) (Team, error) {
	f.created = name
	return Team{ID: 1, Name: name}, nil
}
func (f *fakeStore) Members(_ context.Context, _ int64) ([]Member, error) { return nil, nil }
func (f *fakeStore) Role(_ context.Context, teamID, userID int64) (string, bool, error) {
	if f.roleFn != nil {
		return f.roleFn(teamID, userID)
	}
	return "owner", true, nil
}
func (f *fakeStore) AddMemberByEmail(_ context.Context, _ int64, email string) error {
	f.addEmail = email
	return f.addErr
}
func (f *fakeStore) RemoveMember(_ context.Context, _, _ int64) error  { return nil }
func (f *fakeStore) Rename(_ context.Context, _ int64, _ string) error { return nil }
func (f *fakeStore) Delete(_ context.Context, _ int64) error           { return nil }

func withUser(r *http.Request, uid int64) *http.Request {
	return r.WithContext(auth.WithUserID(r.Context(), uid))
}

func TestListTeams(t *testing.T) {
	f := &fakeStore{listed: []TeamSummary{{ID: 1, Name: "Personal", Personal: true, Role: "owner", MemberCount: 1}}}
	h := NewHandler(f)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, withUser(httptest.NewRequest("GET", "/api/teams", nil), 7))
	if rr.Code != 200 {
		t.Fatalf("code=%d", rr.Code)
	}
	var got []TeamSummary
	json.Unmarshal(rr.Body.Bytes(), &got)
	if len(got) != 1 || got[0].Name != "Personal" {
		t.Fatalf("got %+v", got)
	}
}

func TestCreateTeam(t *testing.T) {
	f := &fakeStore{}
	h := NewHandler(f)
	rr := httptest.NewRecorder()
	body := strings.NewReader(`{"name":"Acme"}`)
	h.ServeHTTP(rr, withUser(httptest.NewRequest("POST", "/api/teams", body), 7))
	if rr.Code != 201 || f.created != "Acme" {
		t.Fatalf("code=%d created=%q", rr.Code, f.created)
	}
}

func TestAddMemberOwnerOnly(t *testing.T) {
	f := &fakeStore{roleFn: func(_, _ int64) (string, bool, error) { return "member", true, nil }}
	h := NewHandler(f)
	rr := httptest.NewRecorder()
	body := strings.NewReader(`{"email":"x@e.com"}`)
	h.ServeHTTP(rr, withUser(httptest.NewRequest("POST", "/api/teams/1/members", body), 7))
	if rr.Code != 403 {
		t.Fatalf("member adding member: code=%d, want 403", rr.Code)
	}
}

func TestAddMemberUnknownEmail404(t *testing.T) {
	f := &fakeStore{addErr: ErrUserNotFound}
	h := NewHandler(f)
	rr := httptest.NewRecorder()
	body := strings.NewReader(`{"email":"nobody@e.com"}`)
	h.ServeHTTP(rr, withUser(httptest.NewRequest("POST", "/api/teams/1/members", body), 7))
	if rr.Code != 404 {
		t.Fatalf("code=%d, want 404", rr.Code)
	}
}
