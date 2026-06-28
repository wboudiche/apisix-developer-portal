package catalog

// Product is a published API in the catalog.
type Product struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	Slug        string   `json:"slug"`
	Category    string   `json:"category"`
	Version     string   `json:"version"`
	ContextPath string   `json:"contextPath"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Icon        string   `json:"icon"`
	Rating      float64  `json:"rating"`
	RatingCount int      `json:"ratingCount"`
}

// Query holds catalog filter/search/sort parameters.
type Query struct {
	Search   string // matches name/description (case-insensitive)
	Category string // exact category, empty = all
	Tag      string // must be present in tags, empty = all
	Sort     string // "alpha" | "rating" (default)
}
