// Package i18n localizes user-facing API messages by request locale.
package i18n

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

type Lang string

const DefaultLang Lang = "fr"

type ctxKey struct{}

func WithLang(ctx context.Context, l Lang) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}

func FromContext(ctx context.Context) Lang {
	if l, ok := ctx.Value(ctxKey{}).(Lang); ok {
		return l
	}
	return DefaultLang
}

// T returns the localized message for key in lang, falling back to French then
// the key itself. When args are given, they are applied with fmt.Sprintf.
func T(lang Lang, key string, args ...any) string {
	cat := fr
	if lang == "en" {
		cat = en
	}
	msg, ok := cat[key]
	if !ok {
		if msg, ok = fr[key]; !ok {
			msg = key
		}
	}
	if len(args) > 0 {
		return fmt.Sprintf(msg, args...)
	}
	return msg
}

// parse reduces an Accept-Language header to a supported Lang.
func parse(header string) Lang {
	tag := strings.TrimSpace(strings.SplitN(strings.SplitN(header, ",", 2)[0], ";", 2)[0])
	if strings.HasPrefix(strings.ToLower(tag), "en") {
		return "en"
	}
	return DefaultLang
}

// Middleware stores the request's resolved locale in the context.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(WithLang(r.Context(), parse(r.Header.Get("Accept-Language")))))
	})
}
