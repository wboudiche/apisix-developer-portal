package settings

import (
	"bufio"
	"context"
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
	return &LiveProber{client: &http.Client{Timeout: probeTimeout}}
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
	d := net.Dialer{Timeout: probeTimeout}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return ProbeResult{Name: "smtp", Detail: err.Error()}
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(probeTimeout))
	r := bufio.NewReader(conn)
	greet, err := r.ReadString('\n')
	if err != nil || !strings.HasPrefix(greet, "220") {
		return ProbeResult{Name: "smtp", Detail: fmt.Sprintf("no SMTP greeting (got %q, err %v)", strings.TrimSpace(greet), err)}
	}
	if _, err := fmt.Fprintf(conn, "EHLO portal\r\n"); err != nil {
		return ProbeResult{Name: "smtp", Detail: err.Error()}
	}
	line, err := r.ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "250") {
		return ProbeResult{Name: "smtp", Detail: fmt.Sprintf("EHLO rejected (got %q, err %v)", strings.TrimSpace(line), err)}
	}
	return ProbeResult{Name: "smtp", OK: true, Detail: "SMTP reachable"}
}
