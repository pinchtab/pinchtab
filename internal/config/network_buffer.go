package config

const MaxNetworkBufferSize = 10000

// ClampNetworkBufferSize bounds per-tab network buffers to a safe maximum.
func ClampNetworkBufferSize(size int) int {
	if size <= 0 {
		return 0
	}
	if size > MaxNetworkBufferSize {
		return MaxNetworkBufferSize
	}
	return size
}
