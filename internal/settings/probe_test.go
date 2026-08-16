package settings

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func candidateWith(t *testing.T, values map[string]string) *Effective {
	t.Helper()
	e := &Effective{Values: map[string]string{}, Source: map[string]string{}}
	for _, d := range registry {
		e.Values[d.Key] = ""
	}
	for k, v := range values {
		e.Values[k] = v
	}
	return e
}

func TestAPISIXProbe(t *testing.T) {
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/apisix/admin/routes") {
			w.WriteHeader(404)
			return
		}
		gotKey = r.Header.Get("X-API-KEY")
		if gotKey != "goodkey" {
			w.WriteHeader(401)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	p := NewProber()
	res := p.Probe(context.Background(), candidateWith(t, map[string]string{
		"APISIX_ADMIN_URL": srv.URL, "APISIX_ADMIN_KEY": "goodkey",
	}), map[string]bool{"APISIX_ADMIN_URL": true})
	if len(res) != 1 || res[0].Name != "apisix" || !res[0].OK {
		t.Fatalf("good key: %+v", res)
	}
	res = p.Probe(context.Background(), candidateWith(t, map[string]string{
		"APISIX_ADMIN_URL": srv.URL, "APISIX_ADMIN_KEY": "badkey",
	}), map[string]bool{"APISIX_ADMIN_KEY": true})
	if len(res) != 1 || res[0].OK {
		t.Fatalf("bad key must fail: %+v", res)
	}
}

func TestSMTPProbe(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = c.Write([]byte("220 test ESMTP\r\n"))
				buf := make([]byte, 128)
				_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
				_, _ = c.Read(buf) // EHLO line
				_, _ = c.Write([]byte("250 ok\r\n"))
			}(c)
		}
	}()
	u, _ := url.Parse("http://" + ln.Addr().String())
	host, port := u.Hostname(), u.Port()

	p := NewProber()
	res := p.Probe(context.Background(), candidateWith(t, map[string]string{
		"SMTP_HOST": host, "SMTP_PORT": port,
	}), map[string]bool{"SMTP_HOST": true})
	if len(res) != 1 || res[0].Name != "smtp" || !res[0].OK {
		t.Fatalf("smtp ok: %+v", res)
	}
	res = p.Probe(context.Background(), candidateWith(t, map[string]string{
		"SMTP_HOST": "127.0.0.1", "SMTP_PORT": "1", // nothing listens on :1
	}), map[string]bool{"SMTP_HOST": true})
	if len(res) != 1 || res[0].OK {
		t.Fatalf("smtp refused must fail: %+v", res)
	}
}

// A server that accepts the connection then says nothing trips the deadline
// AfterFunc sets, surfacing as "i/o timeout" — which reads as the server
// misbehaving. The detail must name our own budget instead.
func TestSMTPProbeTimeoutBlamesTheBudgetNotTheServer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		accepted <- c // held open, never greets
	}()

	u, _ := url.Parse("http://" + ln.Addr().String())
	// Cancel the parent rather than wait out probeTimeout: same code path
	// (AfterFunc expires the conn deadline), no 5s test.
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	p := NewProber()
	res := p.smtp(ctx, u.Hostname(), u.Port())
	if c := <-accepted; c != nil {
		c.Close()
	}
	if res.OK {
		t.Fatalf("silent server must fail the probe: %+v", res)
	}
	if !strings.Contains(res.Detail, "cancelled") {
		t.Fatalf("detail should blame the cancelled probe, got %q", res.Detail)
	}
	if strings.Contains(res.Detail, "i/o timeout") {
		t.Fatalf("detail still leaks the raw deadline error: %q", res.Detail)
	}
}

func TestProbeSkipsUntouchedAndUnconfigured(t *testing.T) {
	p := NewProber()
	// Touching only PORTAL_BASE_URL probes nothing.
	res := p.Probe(context.Background(), candidateWith(t, nil), map[string]bool{"PORTAL_BASE_URL": true})
	if len(res) != 0 {
		t.Fatalf("no probes expected, got %+v", res)
	}
	// Touching sandbox keys while sandbox candidate is empty: skipped-OK.
	res = p.Probe(context.Background(), candidateWith(t, nil), map[string]bool{"APISIX_SANDBOX_ADMIN_URL": true})
	if len(res) != 1 || !res[0].OK || res[0].Name != "sandbox" {
		t.Fatalf("empty sandbox must be skipped-OK: %+v", res)
	}
	// Touching SMTP keys while SMTP_HOST empty: skipped-OK.
	res = p.Probe(context.Background(), candidateWith(t, nil), map[string]bool{"SMTP_FROM": true})
	if len(res) != 1 || !res[0].OK || res[0].Name != "smtp" {
		t.Fatalf("empty smtp must be skipped-OK: %+v", res)
	}
}
