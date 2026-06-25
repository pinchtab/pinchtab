package cdptk

// FilterAnnotationItems keeps only items whose viewport rect overlaps the
// active capture region. For viewport mode `target` is nil and `viewport`
// is used; for selector-clip mode the selector rect is used instead.
func FilterAnnotationItems(items []AnnotationItem, target *AnnotationRect, viewport AnnotationRect) []AnnotationItem {
	region := viewport
	if target != nil {
		region = *target
	}
	out := make([]AnnotationItem, 0, len(items))
	for _, it := range items {
		if RectsOverlap(it.Box, region) {
			out = append(out, it)
		}
	}
	return out
}

func RectsOverlap(a, b AnnotationRect) bool {
	if a.W <= 0 || a.H <= 0 || b.W <= 0 || b.H <= 0 {
		return false
	}
	return a.X < b.X+b.W && a.X+a.W > b.X && a.Y < b.Y+b.H && a.Y+a.H > b.Y
}

// ProjectAnnotationBoxes returns boxes in the coordinate space of the
// returned screenshot. Viewport mode passes the rects through unchanged;
// selector-clip mode subtracts the target origin so boxes are relative to
// the clipped image; beyond-viewport mode adds the current document scroll
// because the image origin is the document top-left, not the viewport.
func ProjectAnnotationBoxes(items []AnnotationItem, target *AnnotationRect, mode CaptureMode, scrollX, scrollY float64) []AnnotationItem {
	out := make([]AnnotationItem, len(items))
	for i, it := range items {
		out[i] = it
		switch {
		case mode == ModeSelectorClip && target != nil:
			out[i].Box = AnnotationRect{
				X: RoundFloat(it.Box.X - target.X),
				Y: RoundFloat(it.Box.Y - target.Y),
				W: RoundFloat(it.Box.W),
				H: RoundFloat(it.Box.H),
			}
		case mode == ModeBeyondViewport:
			out[i].Box = AnnotationRect{
				X: RoundFloat(it.Box.X + scrollX),
				Y: RoundFloat(it.Box.Y + scrollY),
				W: RoundFloat(it.Box.W),
				H: RoundFloat(it.Box.H),
			}
		default:
			out[i].Box = AnnotationRect{
				X: RoundFloat(it.Box.X),
				Y: RoundFloat(it.Box.Y),
				W: RoundFloat(it.Box.W),
				H: RoundFloat(it.Box.H),
			}
		}
	}
	return out
}

// RoundFloat returns the float rounded to the nearest integer (still as
// float64). Spec asks for integer values in the public box payload.
func RoundFloat(f float64) float64 {
	if f >= 0 {
		return float64(int64(f + 0.5))
	}
	return float64(int64(f - 0.5))
}

// RefLess sorts refs like e1 < e2 < e10 by their numeric suffix.
func RefLess(a, b string) bool {
	an := RefNumber(a)
	bn := RefNumber(b)
	if an != bn {
		return an < bn
	}
	return a < b
}

func RefNumber(ref string) int {
	n := 0
	started := false
	for _, c := range ref {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
			started = true
		} else if started {
			break
		}
	}
	return n
}
