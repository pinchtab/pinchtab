package handlers

import (
	"net/url"
	"testing"
)

func TestQueryDecoderValidAndDefaults(t *testing.T) {
	q := url.Values{}
	q.Set("flag", "TRUE")
	q.Set("ratio", "1.5")
	q.Set("count", "42")
	q.Set("big", "9000000000")
	// "missing" / "blank" intentionally absent or empty → keep defaults.
	q.Set("blank", "")

	d := newQueryDecoder(q)
	flag := false
	ratio := 0.0
	count := 0
	var big int64
	missingBool := true // default preserved when absent
	d.Bool("flag", &flag)
	d.Float("ratio", &ratio)
	d.Int("count", &count)
	d.Int64("big", &big)
	d.Bool("missing", &missingBool)
	d.Bool("blank", &missingBool)
	if err := d.Err(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !flag || ratio != 1.5 || count != 42 || big != 9000000000 || !missingBool {
		t.Fatalf("decoded wrong: flag=%v ratio=%v count=%v big=%v missing=%v", flag, ratio, count, big, missingBool)
	}
}

func TestQueryDecoderBoolLiteralSuperset(t *testing.T) {
	// The decoder must accept every truthy literal the old ad-hoc parsers did
	// (case-insensitive true + "1"), so no previously-valid request regresses.
	for _, raw := range []string{"true", "True", "TRUE", "tRuE", "1", "t"} {
		q := url.Values{}
		q.Set("b", raw)
		d := newQueryDecoder(q)
		got := false
		d.Bool("b", &got)
		if err := d.Err(); err != nil || !got {
			t.Fatalf("%q: got=%v err=%v, want true/nil", raw, got, err)
		}
	}
}

func TestQueryDecoderMalformedRejected(t *testing.T) {
	cases := []struct{ key, val string }{
		{"b", "yes"}, // not a bool literal
		{"f", "abc"},
		{"i", "1.5"},
		{"i64", "x"},
	}
	for _, c := range cases {
		q := url.Values{}
		q.Set(c.key, c.val)
		d := newQueryDecoder(q)
		var b bool
		var f float64
		var i int
		var i64 int64
		d.Bool("b", &b)
		d.Float("f", &f)
		d.Int("i", &i)
		d.Int64("i64", &i64)
		if d.Err() == nil {
			t.Fatalf("%s=%q: expected error, got nil", c.key, c.val)
		}
	}
}

func TestQueryTruthyLenient(t *testing.T) {
	// queryTruthy never errors: unrecognized → false (the body-less flag
	// endpoints' contract).
	truthy := []string{"1", "t", "true", "True", "TRUE"}
	falsy := []string{"", "0", "false", "yes", "banana", "2"}
	for _, v := range truthy {
		if !queryTruthy(v) {
			t.Errorf("queryTruthy(%q) = false, want true", v)
		}
	}
	for _, v := range falsy {
		if queryTruthy(v) {
			t.Errorf("queryTruthy(%q) = true, want false", v)
		}
	}
}
