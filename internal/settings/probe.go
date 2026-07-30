package settings

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

const probeTimeout = 5 * time.Second

// LiveProber health-checks candidate settings before they are applied:
// APISIX admin APIs get a 1-item routes list, SMTP gets a dial + EHLO.
type LiveProber struct{ client *http.Client }

func NewProber() *LiveProber {
	// No client-level Timeout: each probe bounds its whole request with a
	// per-request context timeout, the single timeout mechanism.
	return &LiveProber{client: &http.Client{
		// Refuse redirects outright: the APISIX admin-key probe sends the
		// STORED X-API-KEY to an admin-supplied candidate URL, and a 3xx
		// response could otherwise steer that header to an attacker-chosen
		// origin, exfiltrating a write-only secret the admin never intended
		// to share off-target.
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("probe: redirects not allowed")
		},
	}}
}

func groupTouched(touched map[string]bool, group string) bool {
	for k := range touched {
		if d, ok := Lookup(k); ok && d.Group == group {
			return true
		}
	}
	return false
}

func (p *LiveProber) Probe(ctx context.Context, c *Effective, touched map[string]bool) []ProbeResult {
	var out []ProbeResult
	if groupTouched(touched, "apisix") {
		out = append(out, p.apisix(ctx, "apisix", c.Get("APISIX_ADMIN_URL"), c.Get("APISIX_ADMIN_KEY")))
	}
	if groupTouched(touched, "sandbox") {
		if !c.SandboxConfigured() {
			out = append(out, ProbeResult{Name: "sandbox", OK: true, Detail: "sandbox not configured — skipped"})
		} else {
			out = append(out, p.apisix(ctx, "sandbox", c.Get("APISIX_SANDBOX_ADMIN_URL"), c.Get("APISIX_SANDBOX_ADMIN_KEY")))
		}
	}
	if groupTouched(touched, "smtp") {
		// Deliberately checks only SMTP_HOST (not Effective.SMTPConfigured, which
		// also requires SMTP_FROM): the probe tests reachability of the host, and
		// FROM has no bearing on connectivity — probing a half-filled group is
		// useful feedback while the admin is still typing the rest.
		if c.Get("SMTP_HOST") == "" {
			out = append(out, ProbeResult{Name: "smtp", OK: true, Detail: "SMTP not configured — skipped"})
		} else {
			out = append(out, p.smtp(ctx, c.Get("SMTP_HOST"), c.Get("SMTP_PORT")))
		}
	}
	return out
}

func (p *LiveProber) apisix(ctx context.Context, name, adminURL, key string) ProbeResult {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimRight(adminURL, "/")+"/apisix/admin/routes?page_size=1", nil)
	if err != nil {
		return ProbeResult{Name: name, Detail: err.Error()}
	}
	req.Header.Set("X-API-KEY", key)
	resp, err := p.client.Do(req)
	if err != nil {
		return ProbeResult{Name: name, Detail: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ProbeResult{Name: name, Detail: fmt.Sprintf("admin API answered HTTP %d", resp.StatusCode)}
	}
	return ProbeResult{Name: name, OK: true, Detail: "admin API reachable"}
}

func (p *LiveProber) smtp(ctx context.Context, host, port string) ProbeResult {
	if port == "" {
		port = "587"
	}
	// One bound for the whole probe: dial + greeting + EHLO share a single
	// 5s budget (or the parent's shorter deadline).
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return ProbeResult{Name: "smtp", Detail: smtpDetail(ctx, "connect failed", "", err)}
	}
	defer conn.Close()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	// Parent-ctx cancellation interrupts in-flight reads/writes immediately
	// by expiring the conn deadline.
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer stop()
	r := bufio.NewReader(conn)
	greet, err := r.ReadString('\n')
	if err != nil || !strings.HasPrefix(greet, "220") {
		return ProbeResult{Name: "smtp", Detail: smtpDetail(ctx, "no SMTP greeting", greet, err)}
	}
	if _, err := fmt.Fprintf(conn, "EHLO portal\r\n"); err != nil {
		return ProbeResult{Name: "smtp", Detail: smtpDetail(ctx, "EHLO failed", "", err)}
	}
	line, err := r.ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "250") {
		return ProbeResult{Name: "smtp", Detail: smtpDetail(ctx, "EHLO rejected", line, err)}
	}
	return ProbeResult{Name: "smtp", OK: true, Detail: "SMTP reachable"}
}

// smtpDetail explains a failed probe step. When the budget ran out or the
// caller went away, the underlying error is the "i/o timeout" produced by the
// deadline AfterFunc sets — which reads as a fault on the server's side. Say
// whose limit was actually hit instead, and only fall back to the raw error
// when the context is still live.
func smtpDetail(ctx context.Context, step, got string, err error) string {
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return fmt.Sprintf("%s: no response within %s", step, probeTimeout)
	case errors.Is(ctx.Err(), context.Canceled):
		return step + ": probe cancelled"
	case got != "":
		return fmt.Sprintf("%s (got %q, err %v)", step, strings.TrimSpace(got), err)
	default:
		return fmt.Sprintf("%s: %v", step, err)
	}
}
