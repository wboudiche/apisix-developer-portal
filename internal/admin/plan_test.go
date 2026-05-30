package admin

import "testing"

func TestPlanValidate(t *testing.T) {
	base := Plan{Name: "Silver", RateLimit: 100, WindowSeconds: 60}

	cases := []struct {
		name    string
		mutate  func(p *Plan)
		wantErr bool
	}{
		{"valid", func(p *Plan) {}, false},
		{"missing name", func(p *Plan) { p.Name = "" }, true},
		{"blank name", func(p *Plan) { p.Name = "   " }, true},
		{"zero rate", func(p *Plan) { p.RateLimit = 0 }, true},
		{"negative rate", func(p *Plan) { p.RateLimit = -5 }, true},
		{"zero window", func(p *Plan) { p.WindowSeconds = 0 }, true},
		{"negative window", func(p *Plan) { p.WindowSeconds = -1 }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := base
			tc.mutate(&p)
			msg := p.validate()
			if tc.wantErr && msg == "" {
				t.Fatal("expected validation error, got none")
			}
			if !tc.wantErr && msg != "" {
				t.Fatalf("unexpected validation error: %s", msg)
			}
		})
	}
}
