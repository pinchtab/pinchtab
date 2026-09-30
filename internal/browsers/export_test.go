package browsers

func resetForTesting() {
	mu.Lock()
	defer mu.Unlock()
	registry = map[string]Browser{}
}
