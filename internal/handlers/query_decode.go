package handlers

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// queryTruthy reports whether a query value is a recognized boolean-true literal
// (the case-insensitive strconv.ParseBool set: 1/t/true). Unrecognized or empty
// values are false — the lenient contract used by body-less flag endpoints
// (back/forward/reload) that never reject input.
func queryTruthy(v string) bool {
	b, err := strconv.ParseBool(strings.ToLower(strings.TrimSpace(v)))
	return err == nil && b
}

// queryDecoder reads and validates typed values from URL query parameters with
// one consistent parse per kind. A present-but-malformed value records the first
// error so the handler can return a single 400; absent or empty values leave the
// destination untouched, so callers pre-seed their defaults. Recognized boolean
// literals are the case-insensitive strconv.ParseBool set (1/t/true/0/f/false…),
// a superset of the ad-hoc forms the handlers previously accepted.
type queryDecoder struct {
	q   url.Values
	err error
}

func newQueryDecoder(q url.Values) *queryDecoder { return &queryDecoder{q: q} }

// Err returns the first malformed-value error encountered, or nil.
func (d *queryDecoder) Err() error { return d.err }

// present reports whether key carries a non-empty value.
func (d *queryDecoder) present(key string) bool {
	return strings.TrimSpace(d.q.Get(key)) != ""
}

func (d *queryDecoder) fail(key, kind, v string) {
	if d.err == nil {
		d.err = fmt.Errorf("invalid %s value %q for query parameter %q", kind, v, key)
	}
}

// Bool sets *dst from key when present; a malformed value records an error and
// leaves *dst unchanged.
func (d *queryDecoder) Bool(key string, dst *bool) {
	v := strings.TrimSpace(d.q.Get(key))
	if v == "" {
		return
	}
	b, err := strconv.ParseBool(strings.ToLower(v))
	if err != nil {
		d.fail(key, "boolean", v)
		return
	}
	*dst = b
}

// Float sets *dst from key when present; a malformed value records an error.
func (d *queryDecoder) Float(key string, dst *float64) {
	v := strings.TrimSpace(d.q.Get(key))
	if v == "" {
		return
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		d.fail(key, "number", v)
		return
	}
	*dst = n
}

// Int sets *dst from key when present; a malformed value records an error.
func (d *queryDecoder) Int(key string, dst *int) {
	v := strings.TrimSpace(d.q.Get(key))
	if v == "" {
		return
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		d.fail(key, "integer", v)
		return
	}
	*dst = n
}

// Int64 sets *dst from key when present; a malformed value records an error.
func (d *queryDecoder) Int64(key string, dst *int64) {
	v := strings.TrimSpace(d.q.Get(key))
	if v == "" {
		return
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		d.fail(key, "integer", v)
		return
	}
	*dst = n
}
