package paging

import (
	"net/url"
	"testing"
)

func TestParseDefaults(t *testing.T) {
	p := Parse(url.Values{})
	if p.Page != 1 || p.Size != 20 {
		t.Fatalf("defaults: got page=%d size=%d, want 1/20", p.Page, p.Size)
	}
}

func TestParseClampsAndReads(t *testing.T) {
	got := Parse(url.Values{"page": {"3"}, "pageSize": {"50"}})
	if got.Page != 3 || got.Size != 50 {
		t.Fatalf("got %+v, want page=3 size=50", got)
	}
	if c := Parse(url.Values{"pageSize": {"500"}}); c.Size != 100 {
		t.Fatalf("size cap: got %d, want 100", c.Size)
	}
	if c := Parse(url.Values{"page": {"0"}, "pageSize": {"0"}}); c.Page != 1 || c.Size != 20 {
		t.Fatalf("floor: got %+v, want 1/20", c)
	}
	if c := Parse(url.Values{"page": {"abc"}, "pageSize": {"x"}}); c.Page != 1 || c.Size != 20 {
		t.Fatalf("garbage: got %+v, want 1/20", c)
	}
}

func TestLimitOffset(t *testing.T) {
	p := Params{Page: 3, Size: 20}
	if p.Limit() != 20 || p.Offset() != 40 {
		t.Fatalf("got limit=%d offset=%d, want 20/40", p.Limit(), p.Offset())
	}
}

func TestNewNormalizesNilAndMapsFields(t *testing.T) {
	pg := New[int](nil, 7, Params{Page: 2, Size: 20})
	if pg.Items == nil {
		t.Fatal("Items must be non-nil")
	}
	if pg.Total != 7 || pg.Page != 2 || pg.PageSize != 20 {
		t.Fatalf("got %+v", pg)
	}
}
