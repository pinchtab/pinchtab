package profiles

import "errors"

// Sentinel errors for profile mutations. Wrap these with %w when a specific
// name/dir must appear in the user-facing message.
var (
	// ErrProfileExists indicates a profile with the requested name (or its
	// derived directory) already exists. Maps to HTTP 409.
	ErrProfileExists = errors.New("profile already exists")

	// ErrProfileDirExists indicates the destination directory for a profile
	// already exists on disk. Maps to HTTP 409.
	ErrProfileDirExists = errors.New("profile directory already exists")

	// ErrInvalidProfileName indicates the profile name failed validation.
	// Maps to HTTP 400.
	ErrInvalidProfileName = errors.New("invalid profile name")

	// ErrProfileNotFound indicates the requested profile does not exist.
	ErrProfileNotFound = errors.New("profile not found")
)

// taggedError carries a sentinel for errors.Is classification while rendering
// a custom user-facing message, so existing wording is preserved exactly.
type taggedError struct {
	msg      string
	sentinel error
}

func (e *taggedError) Error() string { return e.msg }
func (e *taggedError) Is(target error) bool {
	return target == e.sentinel
}

// tagged returns an error whose message is msg but which matches sentinel under
// errors.Is. Use it where the visible message must stay byte-for-byte identical.
func tagged(sentinel error, msg string) error {
	return &taggedError{msg: msg, sentinel: sentinel}
}
