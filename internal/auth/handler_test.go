package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"apisix-portal/internal/httpx"
	"apisix-portal/internal/i18n"
)

// memUser is the value stored per-email by memRepo.
type memUser struct {
	u         User
	hash      string
	verified  bool
	tokenHash string
	expires   time.Time
}

// memRepo is an in-memory UserStore for handler tests.
type memRepo struct {
	byEmail map[string]*memUser
	nextID  int64
}

func newMemRepo() *memRepo { return &memRepo{byEmail: map[string]*memUser{}} }

func (m *memRepo) Create(_ context.Context, email, hash, name, lang string) (User, error) {
	if _, ok := m.byEmail[email]; ok {
		return User{}, ErrEmailTaken
	}
	m.nextID++
	u := User{ID: m.nextID, Email: email, Name: name, Role: "developer", Language: lang, Verified: true}
	m.byEmail[email] = &memUser{u: u, hash: hash, verified: true}
	return u, nil
}

func (m *memRepo) CreateUnverified(_ context.Context, email, hash, name, lang, tokenHash string, expiresAt time.Time) (User, error) {
	if _, ok := m.byEmail[email]; ok {
		return User{}, ErrEmailTaken
	}
	m.nextID++
	u := User{ID: m.nextID, Email: email, Name: name, Role: "developer", Language: lang}
	m.byEmail[email] = &memUser{u: u, hash: hash, tokenHash: tokenHash, expires: expiresAt}
	return u, nil
}

func (m *memRepo) GetByEmail(_ context.Context, email string) (User, string, error) {
	v, ok := m.byEmail[email]
	if !ok {
		return User{}, "", errors.New("not found")
	}
	u := v.u
	u.Verified = v.verified
	return u, v.hash, nil
}

func (m *memRepo) VerifyByTokenHash(_ context.Context, tokenHash string) error {
	for _, v := range m.byEmail {
		if v.tokenHash == tokenHash && time.Now().Before(v.expires) {
			v.verified, v.tokenHash = true, ""
			return nil
		}
	}
	return ErrTokenInvalid
}

func (m *memRepo) ResetVerifyToken(_ context.Context, email, tokenHash string, expiresAt time.Time) (User, error) {
	v, ok := m.byEmail[email]
	if !ok {
		return User{}, ErrUserNotFound
	}
	if v.verified {
		return User{}, ErrAlreadyVerified
	}
	v.tokenHash, v.expires = tokenHash, expiresAt
	u := v.u
	return u, nil
}

func (m *memRepo) SetLanguage(_ context.Context, userID int64, lang string) error {
	for _, v := range m.byEmail {
		if v.u.ID == userID {
			v.u.Language = lang
			return nil
		}
	}
	return errors.New("not found")
}

func newTestHandler() *Handler {
	return NewHandler(newMemRepo(), NewTokenizer("test-secret"), nil)
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

func (b *brokenRepo) Create(_ context.Context, _, _, _, _ string) (User, error) {
	return User{}, errors.New("db down")
}

func (b *brokenRepo) CreateUnverified(_ context.Context, _, _, _, _, _ string, _ time.Time) (User, error) {
	return User{}, errors.New("db down")
}

func (b *brokenRepo) GetByEmail(_ context.Context, _ string) (User, string, error) {
	return User{}, "", errors.New("db down")
}

func (b *brokenRepo) SetLanguage(_ context.Context, _ int64, _ string) error {
	return errors.New("db down")
}

func (b *brokenRepo) VerifyByTokenHash(_ context.Context, _ string) error {
	return errors.New("db down")
}

func (b *brokenRepo) ResetVerifyToken(_ context.Context, _, _ string, _ time.Time) (User, error) {
	return User{}, errors.New("db down")
}

func TestRegisterDBErrorReturns500(t *testing.T) {
	h := NewHandler(&brokenRepo{}, NewTokenizer("test-secret"), nil)
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

// TestLoginPerAccountRateLimitBlocks verifies that repeated login attempts for
// the same email address are blocked by the per-account limiter, even when the
// requests arrive from different client IPs (defeating per-IP limits alone).
func TestLoginPerAccountRateLimitBlocks(t *testing.T) {
	rl := httpx.NewRateLimiter(2, 0) // burst 2, no refill
	h := NewHandler(newMemRepo(), NewTokenizer("test-secret"), rl)

	login := func(remoteAddr string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login",
			strings.NewReader(`{"email":"victim@example.com","password":"pw123456"}`))
		req.RemoteAddr = remoteAddr
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr.Code
	}

	// First two attempts (from different IPs) consume the burst — they reach the
	// store and get 401 (no such account), which is the expected auth failure.
	for i, addr := range []string{"10.0.0.1:1", "10.0.0.2:1"} {
		code := login(addr)
		if code != http.StatusUnauthorized {
			t.Fatalf("attempt %d from %s: got %d, want 401", i+1, addr, code)
		}
	}

	// Third attempt from yet another IP must be blocked by the per-email limiter.
	if code := login("10.0.0.3:1"); code != http.StatusTooManyRequests {
		t.Fatalf("3rd attempt: got %d, want 429", code)
	}
}

// TestRegisterSeedsLanguageFromAcceptLanguage verifies that register seeds the
// new user's stored language from the request locale resolved by the i18n
// middleware (via i18n.FromContext), not by re-parsing Accept-Language itself.
func TestRegisterSeedsLanguageFromAcceptLanguage(t *testing.T) {
	h := newTestHandler()

	req := httptest.NewRequest(http.MethodPost, "/api/auth/register",
		strings.NewReader(`{"email":"seed@x.io","password":"password1","name":"Ada"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Language", "en")
	// The handler test bypasses the real i18n.Middleware, so set the context
	// locale explicitly — in production the outermost middleware does this.
	req = req.WithContext(i18n.WithLang(req.Context(), "en"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body)
	}
	var out struct {
		User struct {
			Language string `json:"language"`
		} `json:"user"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.User.Language != "en" {
		t.Fatalf("seeded language=%q, want en", out.User.Language)
	}
}

// TestPutLanguage verifies PUT /api/me/language persists a valid "fr"/"en"
// preference for the authenticated user, rejects other values with 400, and
// rejects unauthenticated requests with 401.
func TestPutLanguage(t *testing.T) {
	h := newTestHandler()
	// memRepo.SetLanguage looks the user up by id, so seed one via register
	// first (unlike the real Repo, whose UPDATE is a no-op on unknown ids).
	regRec := httptest.NewRecorder()
	h.ServeHTTP(regRec, httptest.NewRequest(http.MethodPost, "/api/auth/register",
		strings.NewReader(`{"email":"lang@x.io","password":"pw123456","name":"L"}`)))
	var reg struct {
		User struct {
			ID int64 `json:"id"`
		} `json:"user"`
	}
	if err := json.Unmarshal(regRec.Body.Bytes(), &reg); err != nil {
		t.Fatalf("unmarshal register response: %v", err)
	}
	uid := reg.User.ID

	call := func(body string, withUser bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/api/me/language", strings.NewReader(body))
		if withUser {
			req = req.WithContext(WithUserID(req.Context(), uid))
		}
		rec := httptest.NewRecorder()
		h.PutLanguage(rec, req)
		return rec
	}
	if rec := call(`{"language":"en"}`, true); rec.Code != http.StatusNoContent {
		t.Fatalf("valid put code=%d", rec.Code)
	}
	if rec := call(`{"language":"de"}`, true); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad value code=%d, want 400", rec.Code)
	}
	if rec := call(`{"language":"fr"}`, false); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no-user code=%d, want 401", rec.Code)
	}
}

// fakeVerifSender records sends; mutex-guarded because sendVerification
// delivers from a goroutine. waitFor polls until the async send lands.
type fakeVerifSender struct {
	mu   sync.Mutex
	to   string
	body string
	n    int
}

func (f *fakeVerifSender) Send(_ context.Context, to []string, _ string, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.n++
	f.to = strings.Join(to, ",")
	f.body = body
	return nil
}

func (f *fakeVerifSender) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.n
}

func (f *fakeVerifSender) last() (to, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.to, f.body
}

// waitFor blocks until the send count reaches n (polling every 10ms, up to
// 2s), failing the test on timeout.
func (f *fakeVerifSender) waitFor(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if f.count() >= n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d sends (got %d)", n, f.count())
}

func verifHandler(store UserStore, sender *fakeVerifSender) *Handler {
	h := NewHandler(store, NewTokenizer("test-secret"), nil)
	h.EnableEmailVerification(VerificationConfig{
		Sender:  sender,
		BaseURL: "http://localhost:8088",
		GenToken: func() (string, string) {
			return "fixedtoken", HashVerifyToken("fixedtoken")
		},
	})
	return h
}

func postAuth(h *Handler, path string, body any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(b)))
	req = req.WithContext(i18n.WithLang(req.Context(), i18n.Lang("en")))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRegisterWithVerificationWithholdsToken(t *testing.T) {
	sender := &fakeVerifSender{}
	h := verifHandler(newMemRepo(), sender)
	rec := postAuth(h, "/api/auth/register", credentials{Email: "d@x.io", Password: "longenough", Name: "D"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body %s)", rec.Code, rec.Body.String())
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if _, hasToken := body["token"]; hasToken {
		t.Fatal("register must NOT return a token when verification is required")
	}
	if body["verificationRequired"] != true {
		t.Fatal("response must carry verificationRequired: true")
	}
	sender.waitFor(t, 1) // send is async — wait for the goroutine to deliver
	to, mailBody := sender.last()
	if to != "d@x.io" {
		t.Fatalf("verification email not sent to registrant (to=%q)", to)
	}
	if !strings.Contains(mailBody, "http://localhost:8088/verify-email?token=fixedtoken") {
		t.Fatalf("email body must contain the link, got %q", mailBody)
	}
}

func TestRegisterWithoutVerificationKeepsOldBehavior(t *testing.T) {
	h := NewHandler(newMemRepo(), NewTokenizer("test-secret"), nil)
	rec := postAuth(h, "/api/auth/register", credentials{Email: "d@x.io", Password: "longenough"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["token"] == nil || body["token"] == "" {
		t.Fatal("feature off: register must still auto-login with a token")
	}
}

func TestLoginBlockedUntilVerified(t *testing.T) {
	sender := &fakeVerifSender{}
	store := newMemRepo()
	h := verifHandler(store, sender)
	postAuth(h, "/api/auth/register", credentials{Email: "d@x.io", Password: "longenough"})

	rec := postAuth(h, "/api/auth/login", credentials{Email: "d@x.io", Password: "longenough"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unverified login status = %d, want 403 (body %s)", rec.Code, rec.Body.String())
	}

	rec = postAuth(h, "/api/auth/verify", map[string]string{"token": "fixedtoken"})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("verify status = %d, want 204 (body %s)", rec.Code, rec.Body.String())
	}

	rec = postAuth(h, "/api/auth/login", credentials{Email: "d@x.io", Password: "longenough"})
	if rec.Code != http.StatusOK {
		t.Fatalf("verified login status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
}

func TestVerifyBadTokenReturns410(t *testing.T) {
	h := verifHandler(newMemRepo(), &fakeVerifSender{})
	rec := postAuth(h, "/api/auth/verify", map[string]string{"token": "nope"})
	if rec.Code != http.StatusGone {
		t.Fatalf("status = %d, want 410", rec.Code)
	}
}

func TestResendAlways204AndOnlyMailsUnverified(t *testing.T) {
	sender := &fakeVerifSender{}
	store := newMemRepo()
	h := verifHandler(store, sender)
	postAuth(h, "/api/auth/register", credentials{Email: "d@x.io", Password: "longenough"})
	sender.waitFor(t, 1) // register's async send must land before counting
	base := sender.count()

	// unknown account: 204, no email. Deterministic even with the async send:
	// ResetVerifyToken errors first, so no goroutine is ever spawned, and every
	// earlier positive send was already waited on above.
	rec := postAuth(h, "/api/auth/resend-verification", map[string]string{"email": "ghost@x.io"})
	if rec.Code != http.StatusNoContent || sender.count() != base {
		t.Fatalf("unknown: code=%d mails=%d, want 204 and no mail", rec.Code, sender.count()-base)
	}
	// unverified account: 204 + email
	rec = postAuth(h, "/api/auth/resend-verification", map[string]string{"email": "d@x.io"})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("unverified: code=%d, want 204", rec.Code)
	}
	sender.waitFor(t, base+1)
	// verify, then resend: 204, no new email (no goroutine spawned — see above)
	postAuth(h, "/api/auth/verify", map[string]string{"token": "fixedtoken"})
	rec = postAuth(h, "/api/auth/resend-verification", map[string]string{"email": "d@x.io"})
	if rec.Code != http.StatusNoContent || sender.count() != base+1 {
		t.Fatalf("verified: code=%d mails=%d, want 204 and no mail", rec.Code, sender.count()-base-1)
	}
}

// TestResendVerificationRateLimited verifies that repeated resend requests for
// the same email are throttled by VerificationConfig.Limiter: the first
// request goes through (204) and the second is blocked (429), mirroring the
// per-account login limiter test above.
func TestResendVerificationRateLimited(t *testing.T) {
	sender := &fakeVerifSender{}
	store := newMemRepo()
	h := NewHandler(store, NewTokenizer("test-secret"), nil)
	h.EnableEmailVerification(VerificationConfig{
		Sender:  sender,
		BaseURL: "http://localhost:8088",
		Limiter: httpx.NewRateLimiter(1, 0), // burst 1, no refill
		GenToken: func() (string, string) {
			return "fixedtoken", HashVerifyToken("fixedtoken")
		},
	})
	postAuth(h, "/api/auth/register", credentials{Email: "d@x.io", Password: "longenough"})
	sender.waitFor(t, 1) // register's async send must land before counting

	rec := postAuth(h, "/api/auth/resend-verification", map[string]string{"email": "d@x.io"})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("first resend: code=%d, want 204 (body %s)", rec.Code, rec.Body.String())
	}
	rec = postAuth(h, "/api/auth/resend-verification", map[string]string{"email": "d@x.io"})
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second resend: code=%d, want 429 (body %s)", rec.Code, rec.Body.String())
	}
}

func TestVerifyRoutesAbsentWhenDisabled(t *testing.T) {
	h := NewHandler(newMemRepo(), NewTokenizer("test-secret"), nil)
	rec := postAuth(h, "/api/auth/verify", map[string]string{"token": "x"})
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("feature off: verify route must not exist, got %d", rec.Code)
	}
}

type toggleProvider struct {
	on      bool
	baseURL string
}

func (p *toggleProvider) VerificationEnabled() bool   { return p.on }
func (p *toggleProvider) VerificationBaseURL() string { return p.baseURL }

func TestDynamicVerificationToggles(t *testing.T) {
	sender := &fakeVerifSender{}
	prov := &toggleProvider{on: false, baseURL: "http://localhost:8088"}
	h := NewHandler(newMemRepo(), NewTokenizer("test-secret"), nil)
	h.EnableDynamicVerification(prov, sender, nil)

	// OFF: routes answer 404; register auto-logins like the feature-off path.
	rec := postAuth(h, "/api/auth/verify", map[string]string{"token": "x"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("verify while off = %d, want 404", rec.Code)
	}
	rec = postAuth(h, "/api/auth/register", credentials{Email: "a@x.io", Password: "longenough"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("register off = %d", rec.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["token"] == nil {
		t.Fatal("off: register must return a token")
	}

	// ON: register withholds token, sends mail; verify works.
	prov.on = true
	rec = postAuth(h, "/api/auth/register", credentials{Email: "b@x.io", Password: "longenough"})
	body = map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if _, has := body["token"]; has {
		t.Fatal("on: register must withhold the token")
	}
	sender.waitFor(t, 1)

	// Back OFF mid-flight: an unverified user can now log in (gate consults
	// the provider per request).
	prov.on = false
	rec = postAuth(h, "/api/auth/login", credentials{Email: "b@x.io", Password: "longenough"})
	if rec.Code != http.StatusOK {
		t.Fatalf("login with gate re-disabled = %d, want 200", rec.Code)
	}
}
