package i18n

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTFallbackAndInterp(t *testing.T) {
	fr["test.hi"] = "Bonjour %s"
	en["test.hi"] = "Hello %s"
	if got := T("en", "test.hi", "Ada"); got != "Hello Ada" {
		t.Errorf("en = %q", got)
	}
	if got := T("fr", "test.hi", "Ada"); got != "Bonjour Ada" {
		t.Errorf("fr = %q", got)
	}
	// unknown lang falls back to fr; unknown key falls back to the key itself
	if got := T("de", "test.hi", "Ada"); got != "Bonjour Ada" {
		t.Errorf("fallback lang = %q", got)
	}
	if got := T("en", "no.such.key"); got != "no.such.key" {
		t.Errorf("missing key = %q", got)
	}
	delete(fr, "test.hi")
	delete(en, "test.hi")
}

func TestParity(t *testing.T) {
	for k := range fr {
		if _, ok := en[k]; !ok {
			t.Errorf("key %q in fr but not en", k)
		}
	}
	for k := range en {
		if _, ok := fr[k]; !ok {
			t.Errorf("key %q in en but not fr", k)
		}
	}
}

func TestMiddlewareResolvesLocale(t *testing.T) {
	cases := map[string]Lang{"fr": "fr", "en-US,en;q=0.9": "en", "": "fr", "de-DE": "fr"}
	for header, want := range cases {
		var got Lang
		h := Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got = FromContext(r.Context()) }))
		req := httptest.NewRequest("GET", "/", nil)
		if header != "" {
			req.Header.Set("Accept-Language", header)
		}
		h.ServeHTTP(httptest.NewRecorder(), req)
		if got != want {
			t.Errorf("Accept-Language %q → %q, want %q", header, got, want)
		}
	}
}

func TestFromContextDefault(t *testing.T) {
	if FromContext(context.Background()) != DefaultLang {
		t.Error("empty context should yield DefaultLang")
	}
}
