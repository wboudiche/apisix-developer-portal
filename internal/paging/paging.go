// Package paging provides offset/page-based pagination primitives shared by the
// list endpoints: request parsing, a Params value, and a JSON response envelope.
package paging

import (
	"net/url"
	"strconv"
)

const (
	DefaultPage     = 1
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// Params is a validated page request.
type Params struct {
	Page int // >= 1
	Size int // 1..MaxPageSize
}

// Limit is the SQL LIMIT for this page.
func (p Params) Limit() int { return p.Size }

// Offset is the SQL OFFSET for this page.
func (p Params) Offset() int { return (p.Page - 1) * p.Size }

// Parse reads ?page and ?pageSize, applying defaults, floors, and the size cap.
// Non-numeric or out-of-range values fall back to defaults.
func Parse(v url.Values) Params {
	page := DefaultPage
	if n, err := strconv.Atoi(v.Get("page")); err == nil && n >= 1 {
		page = n
	}
	size := DefaultPageSize
	if n, err := strconv.Atoi(v.Get("pageSize")); err == nil && n >= 1 {
		size = n
	}
	if size > MaxPageSize {
		size = MaxPageSize
	}
	return Params{Page: page, Size: size}
}

// Page is the JSON envelope returned by paginated list endpoints. Items is
// always a non-nil slice so the JSON renders "items": [] rather than null.
type Page[T any] struct {
	Items    []T `json:"items"`
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
}

// New builds a Page envelope, normalizing a nil items slice to empty.
func New[T any](items []T, total int, p Params) Page[T] {
	if items == nil {
		items = []T{}
	}
	return Page[T]{Items: items, Total: total, Page: p.Page, PageSize: p.Size}
}
