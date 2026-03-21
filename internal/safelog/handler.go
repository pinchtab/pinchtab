package safelog

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"unicode"

	"github.com/pinchtab/pinchtab/internal/sanitize"
)

const (
	MaxStringValueBytes = 2 * 1024
	MaxRecordTextBytes  = 8 * 1024
	redactedValue       = "[REDACTED]"
)

type Handler struct {
	next slog.Handler
}

var installOnce sync.Once

func NewHandler(next slog.Handler) *Handler {
	if next == nil {
		next = slog.Default().Handler()
	}
	return &Handler{next: next}
}

func InstallDefault() {
	installOnce.Do(func() {
		base := slog.NewTextHandler(os.Stderr, nil)
		slog.SetDefault(slog.New(NewHandler(base)))
	})
}

func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *Handler) Handle(ctx context.Context, rec slog.Record) error {
	budget := MaxRecordTextBytes
	msg, budget := sanitizeText(rec.Message, budget)
	out := slog.NewRecord(rec.Time, rec.Level, msg, rec.PC)

	rec.Attrs(func(attr slog.Attr) bool {
		sanitizedAttr, nextBudget := sanitizeAttr(attr, budget)
		budget = nextBudget
		out.AddAttrs(sanitizedAttr)
		return true
	})

	return h.next.Handle(ctx, out)
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	budget := MaxRecordTextBytes
	sanitizedAttrs := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		sanitizedAttr, nextBudget := sanitizeAttr(attr, budget)
		budget = nextBudget
		sanitizedAttrs = append(sanitizedAttrs, sanitizedAttr)
	}
	return &Handler{next: h.next.WithAttrs(sanitizedAttrs)}
}

func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{next: h.next.WithGroup(name)}
}

func sanitizeAttr(attr slog.Attr, budget int) (slog.Attr, int) {
	attr.Value = attr.Value.Resolve()
	if isSensitiveKey(attr.Key) {
		return slog.String(attr.Key, redactedValue), clampBudget(budget - len(redactedValue))
	}

	switch attr.Value.Kind() {
	case slog.KindString:
		s, nextBudget := sanitizeText(attr.Value.String(), budget)
		return slog.String(attr.Key, s), nextBudget
	case slog.KindGroup:
		group := attr.Value.Group()
		sanitizedGroup := make([]slog.Attr, 0, len(group))
		for _, child := range group {
			sanitizedChild, nextBudget := sanitizeAttr(child, budget)
			budget = nextBudget
			sanitizedGroup = append(sanitizedGroup, sanitizedChild)
		}
		return slog.Attr{Key: attr.Key, Value: slog.GroupValue(sanitizedGroup...)}, budget
	case slog.KindAny:
		switch v := attr.Value.Any().(type) {
		case error:
			s, nextBudget := sanitizeText(v.Error(), budget)
			return slog.String(attr.Key, s), nextBudget
		case fmt.Stringer:
			s, nextBudget := sanitizeText(v.String(), budget)
			return slog.String(attr.Key, s), nextBudget
		case string:
			s, nextBudget := sanitizeText(v, budget)
			return slog.String(attr.Key, s), nextBudget
		case []byte:
			s, nextBudget := sanitizeText(string(v), budget)
			return slog.String(attr.Key, s), nextBudget
		}
	}

	return attr, budget
}

func sanitizeText(s string, budget int) (string, int) {
	limit := MaxStringValueBytes
	if budget < limit {
		limit = budget
	}
	if limit < 0 {
		limit = 0
	}
	s = sanitize.CleanForLog(s, limit)
	return s, clampBudget(budget - len(s))
}

func clampBudget(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

func isSensitiveKey(key string) bool {
	var b strings.Builder
	b.Grow(len(key))
	for _, r := range key {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	normalized := b.String()
	if normalized == "" {
		return false
	}
	return strings.Contains(normalized, "password") ||
		strings.Contains(normalized, "cookie") ||
		strings.Contains(normalized, "authorization") ||
		normalized == "token" ||
		strings.HasSuffix(normalized, "token")
}
