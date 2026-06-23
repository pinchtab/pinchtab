package dashboard

import (
	"reflect"
	"testing"
)

func TestRingBuffer_PushSnapshotOrder(t *testing.T) {
	r := newRingBuffer[int](3)
	if r.len() != 0 {
		t.Fatalf("empty len = %d, want 0", r.len())
	}
	if got := r.snapshot(); len(got) != 0 {
		t.Fatalf("empty snapshot = %v, want []", got)
	}

	// Partial fill: oldest -> newest.
	r.push(1)
	r.push(2)
	if got := r.snapshot(); !reflect.DeepEqual(got, []int{1, 2}) {
		t.Fatalf("partial snapshot = %v, want [1 2]", got)
	}
	if r.len() != 2 {
		t.Fatalf("len = %d, want 2", r.len())
	}
}

func TestRingBuffer_EvictsOldestWhenFull(t *testing.T) {
	r := newRingBuffer[int](3)
	for _, v := range []int{1, 2, 3} {
		if _, didEvict := r.push(v); didEvict {
			t.Fatalf("push(%d) evicted before full", v)
		}
	}
	// Buffer full with [1 2 3]; next push evicts 1.
	evicted, didEvict := r.push(4)
	if !didEvict || evicted != 1 {
		t.Fatalf("push(4) = (%d,%v), want (1,true)", evicted, didEvict)
	}
	if got := r.snapshot(); !reflect.DeepEqual(got, []int{2, 3, 4}) {
		t.Fatalf("snapshot = %v, want [2 3 4]", got)
	}
	// Wrap further: newest 3 retained, oldest->newest order preserved.
	r.push(5)
	r.push(6)
	if got := r.snapshot(); !reflect.DeepEqual(got, []int{4, 5, 6}) {
		t.Fatalf("snapshot = %v, want [4 5 6]", got)
	}
	if r.len() != 3 {
		t.Fatalf("len = %d, want 3 (capped)", r.len())
	}
}

func TestRingBuffer_ForEachOrder(t *testing.T) {
	r := newRingBuffer[string](2)
	r.push("a")
	r.push("b")
	r.push("c") // evicts "a"
	var seen []string
	r.forEach(func(s string) { seen = append(seen, s) })
	if !reflect.DeepEqual(seen, []string{"b", "c"}) {
		t.Fatalf("forEach order = %v, want [b c]", seen)
	}
}
