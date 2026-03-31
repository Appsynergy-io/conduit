package middleware

import (
	"context"

	"github.com/appsynergy-io/conduit/internal/auth"
)

// contextKey is an unexported type for context keys to prevent collisions.
type contextKey int

const (
	keyRequestID contextKey = iota
	keyClaims
	keyTenantID
)

// WithRequestID stores the request ID in context.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyRequestID, id)
}

// RequestIDFromCtx extracts the request ID from context.
func RequestIDFromCtx(ctx context.Context) string {
	id, _ := ctx.Value(keyRequestID).(string)
	return id
}

// WithClaims stores JWT claims in context.
func WithClaims(ctx context.Context, claims *auth.Claims) context.Context {
	return context.WithValue(ctx, keyClaims, claims)
}

// ClaimsFromCtx extracts JWT claims from context.
func ClaimsFromCtx(ctx context.Context) *auth.Claims {
	claims, _ := ctx.Value(keyClaims).(*auth.Claims)
	return claims
}

// UserFromCtx returns the user ID from JWT claims in context.
func UserFromCtx(ctx context.Context) string {
	claims := ClaimsFromCtx(ctx)
	if claims == nil {
		return ""
	}
	return claims.Subject
}

// TenantIDFromCtx returns the tenant ID from JWT claims in context.
func TenantIDFromCtx(ctx context.Context) string {
	claims := ClaimsFromCtx(ctx)
	if claims == nil {
		id, _ := ctx.Value(keyTenantID).(string)
		return id
	}
	return claims.TenantID
}

// WithTenantID stores the tenant ID in context (used when not from JWT).
func WithTenantID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, keyTenantID, id)
}
