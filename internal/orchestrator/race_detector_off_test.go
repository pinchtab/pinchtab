//go:build !race

package orchestrator

// raceDetectorEnabled reports whether this test binary was built with -race.
// See race_detector_on_test.go.
const raceDetectorEnabled = false
