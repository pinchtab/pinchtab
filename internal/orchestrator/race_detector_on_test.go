//go:build race

package orchestrator

// raceDetectorEnabled reports whether this test binary was built with -race.
// The launch-response test asserts nothing itself — the detector is its
// assertion — so it skips rather than passing vacuously when this is false.
const raceDetectorEnabled = true
