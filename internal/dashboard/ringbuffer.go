package dashboard

// ringBuffer is a fixed-capacity FIFO with O(1) push and eviction of the oldest
// element when full, avoiding the O(n) slice shifts of an append/reslice queue.
type ringBuffer[T any] struct {
	buf  []T
	head int // index of the oldest element
	size int // current count (<= len(buf))
}

func newRingBuffer[T any](capacity int) *ringBuffer[T] {
	if capacity < 1 {
		capacity = 1
	}
	return &ringBuffer[T]{buf: make([]T, capacity)}
}

func (r *ringBuffer[T]) len() int { return r.size }

// push stores v; when full it overwrites and returns the evicted oldest element.
func (r *ringBuffer[T]) push(v T) (evicted T, didEvict bool) {
	if r.size < len(r.buf) {
		r.buf[(r.head+r.size)%len(r.buf)] = v
		r.size++
		return
	}
	evicted, didEvict = r.buf[r.head], true
	r.buf[r.head] = v
	r.head = (r.head + 1) % len(r.buf)
	return
}

// snapshot returns the elements oldest to newest.
func (r *ringBuffer[T]) snapshot() []T {
	out := make([]T, r.size)
	for i := 0; i < r.size; i++ {
		out[i] = r.buf[(r.head+i)%len(r.buf)]
	}
	return out
}

// forEach iterates the elements oldest to newest.
func (r *ringBuffer[T]) forEach(fn func(T)) {
	for i := 0; i < r.size; i++ {
		fn(r.buf[(r.head+i)%len(r.buf)])
	}
}
