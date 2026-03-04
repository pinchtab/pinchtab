package bridge

import "fmt"

// TabLimitError indicates that the tab limit has been reached.
// It implements error and provides a StatusCode() method for HTTP responses.
type TabLimitError struct {
	Current int
	Max     int
}

func (e TabLimitError) Error() string {
	return fmt.Sprintf("tab limit reached (%d/%d)", e.Current, e.Max)
}

// StatusCode returns the HTTP status code for this error (429 Too Many Requests)
func (e TabLimitError) StatusCode() int {
	return 429
}

// IsTabLimitError checks if an error is a TabLimitError
func IsTabLimitError(err error) bool {
	_, ok := err.(TabLimitError)
	return ok
}
