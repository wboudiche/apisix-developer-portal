package ratings

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
	mine    *Review
	items   []Review
	summary Summary
	gotPut  *putArgs
}

type putArgs struct {
	productID, userID int64
	stars             int
	comment           string
}

func (f *fakeStore) Upsert(_ context.Context, p, u int64, s int, c string) error {
	f.gotPut = &putArgs{p, u, s, c}
	f.summary = Summary{Average: float64(s), Count: 1}
	f.mine = &Review{Stars: s, Comment: c, Author: "Me"}
	return nil
}
func (f *fakeStore) List(context.Context, int64) ([]Review, error)       { return f.items, nil }
func (f *fakeStore) Mine(context.Context, int64, int64) (*Review, error) { return f.mine, nil }
func (f *fakeStore) SummaryFor(context.Context, int64) (Summary, error)  { return f.summary, nil }

type fakeProducts struct {
	id  int64
	err error
}

func (f fakeProducts) ProductBySlug(context.Context, string) (int64, error) { return f.id, f.err }

type fakeSubs struct{ approved bool }

func (f fakeSubs) IsApprovedSubscriber(context.Context, int64, int64) (bool, error) {
	return f.approved, nil
}

func testTok(t *testing.T) (*auth.Tokenizer, string) {
	t.Helper()
	tok := auth.NewTokenizer("test-secret")
	s, err := tok.Issue(7, "dev@example.com", "developer")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	return tok, s
}

func TestRatingsGetPublic(t *testing.T) {
	tok, _ := testTok(t)
	h := NewHandler(&fakeStore{summary: Summary{Average: 4, Count: 2}, items: []Review{{Stars: 4, Author: "A"}}},
		fakeProducts{id: 9}, fakeSubs{}, tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/ratings/orders", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	var v RatingsView
	_ = json.Unmarshal(rec.Body.Bytes(), &v)
	if v.Average != 4 || v.Count != 2 || len(v.Items) != 1 || v.CanRate {
		t.Fatalf("view=%+v", v)
	}
}

func TestRatingsPutApprovedSubscriber(t *testing.T) {
	tok, jwt := testTok(t)
	store := &fakeStore{}
	h := NewHandler(store, fakeProducts{id: 9}, fakeSubs{approved: true}, tok)
	req := httptest.NewRequest(http.MethodPut, "/api/ratings/orders", strings.NewReader(`{"stars":5,"comment":"top"}`))
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
	if store.gotPut == nil || store.gotPut.stars != 5 || store.gotPut.userID != 7 {
		t.Fatalf("put=%+v", store.gotPut)
	}
}

func TestRatingsPutNonSubscriber403(t *testing.T) {
	tok, jwt := testTok(t)
	h := NewHandler(&fakeStore{}, fakeProducts{id: 9}, fakeSubs{approved: false}, tok)
	req := httptest.NewRequest(http.MethodPut, "/api/ratings/orders", strings.NewReader(`{"stars":5}`))
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestRatingsPutBadStars400(t *testing.T) {
	tok, jwt := testTok(t)
	h := NewHandler(&fakeStore{}, fakeProducts{id: 9}, fakeSubs{approved: true}, tok)
	req := httptest.NewRequest(http.MethodPut, "/api/ratings/orders", strings.NewReader(`{"stars":9}`))
	req.Header.Set("Authorization", "Bearer "+jwt)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestRatingsPutAnon401(t *testing.T) {
	tok, _ := testTok(t)
	h := NewHandler(&fakeStore{}, fakeProducts{id: 9}, fakeSubs{approved: true}, tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/ratings/orders", strings.NewReader(`{"stars":5}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", rec.Code)
	}
}
