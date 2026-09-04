package runtime

import "runtime"

// exeSuffix is what this platform requires on the end of an executable's name.
// Windows decides executability by extension, so a stub without one is not a
// browser as far as discovery is concerned.
var exeSuffix = func() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}()
