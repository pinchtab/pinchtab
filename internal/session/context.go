package session

import (
	"context"
	"net/http"
)

type contextKey struct{}

// WithSession stores an authenticated session on the request context.
func WithSession(r *http.Request, sess *Session) *http.Request {
	if r == nil || sess == nil {
		return r
	}
	return r.WithContext(context.WithValue(r.Context(), contextKey{}, sess))
}

// FromRequest returns the authenticated session stored on the request context.
func FromRequest(r *http.Request) (*Session, bool) {
	if r == nil {
		return nil, false
	}
	sess, ok := r.Context().Value(contextKey{}).(*Session)
	return sess, ok && sess != nil
}
