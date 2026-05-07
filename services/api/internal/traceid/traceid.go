package traceid

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
)

const Header = "X-Trace-ID"

type contextKey struct{}

func New() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "trc_fallback"
	}
	return "trc_" + hex.EncodeToString(bytes[:])
}

func Ensure(value string) string {
	value = Normalize(value)
	if value == "" {
		return New()
	}
	return value
}

func Normalize(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return ""
	}
	for _, r := range value {
		if r < 33 || r > 126 {
			return ""
		}
	}
	return value
}

func WithContext(ctx context.Context, value string) context.Context {
	return context.WithValue(ctx, contextKey{}, Ensure(value))
}

func FromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(contextKey{}).(string)
	return value
}
