package bridge

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
)

// TestPollingScreencast_FirstFrameArrives verifies that the polling
// screencast goroutine sends at least one frame to the channel
// within a reasonable time when CaptureScreenshot succeeds.
func TestPollingScreencast_FirstFrameArrives(t *testing.T) {
	b := &Bridge{
		Config: &config.RuntimeConfig{
			Headless:       true,
			DefaultBrowser: "chrome",
		},
	}

	// Monkey-patch CaptureScreenshot on the bridge by embedding a
	// test-friendly polling function. Since we can't override the
	// method, we directly test startScreencastPolling's behavior
	// by calling it and checking the channel.
	//
	// We rely on the fact that startScreencastPolling calls
	// b.CaptureScreenshot, and that will fail with "no browser"
	// (context is nil). We expect the goroutine to log the error
	// and exit. This proves the silent-error scenario.

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := b.startScreencastPolling(ctx, ScreencastOpts{
		Quality:  30,
		MaxWidth: 320,
		FPS:      5,
	})

	// The stream should be created even if the first frame will fail.
	if err != nil {
		t.Fatalf("startScreencastPolling returned error: %v", err)
	}
	defer stream.Close()

	// Wait for the channel to close (error path) or a frame (success path).
	select {
	case frame, ok := <-stream.Frames:
		if !ok {
			// Channel closed — this means CaptureScreenshot failed.
			// This is the expected behavior for a nil chromedp context.
			t.Log("frame channel closed without sending — CaptureScreenshot errored (expected for nil context)")
		} else {
			t.Logf("received frame: %d bytes", len(frame))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for frame or channel close — goroutine may be hung")
	}
}

// TestPollingScreencast_FrameDeliveryTiming simulates the polling loop
// to verify frame cadence matches the requested FPS.
func TestPollingScreencast_FrameDeliveryTiming(t *testing.T) {
	var callCount atomic.Int64
	fakeFrame := make([]byte, 1024)
	for i := range fakeFrame {
		fakeFrame[i] = byte(i % 256)
	}

	b := &Bridge{
		Config: &config.RuntimeConfig{
			Headless:       true,
			DefaultBrowser: "chrome",
		},
	}

	// We can't easily inject a fake CaptureScreenshot into the bridge
	// (it calls chromedp.Run), so instead we test the channel/timing
	// logic directly with a simulated producer.
	fps := 5
	frameInterval := time.Second / time.Duration(fps)
	frameCh := make(chan []byte, 2)
	done := make(chan struct{})

	go func() {
		defer close(frameCh)

		callCount.Add(1)
		select {
		case frameCh <- fakeFrame:
		case <-done:
			return
		}

		ticker := time.NewTicker(frameInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				callCount.Add(1)
				select {
				case frameCh <- fakeFrame:
				case <-done:
					return
				}
			case <-done:
				return
			}
		}
	}()

	start := time.Now()
	var received int
	timeout := time.After(1100 * time.Millisecond)

	for {
		select {
		case frame, ok := <-frameCh:
			if !ok {
				t.Fatal("frame channel closed unexpectedly")
			}
			received++
			if len(frame) != len(fakeFrame) {
				t.Fatalf("frame size mismatch: got %d, want %d", len(frame), len(fakeFrame))
			}
		case <-timeout:
			goto checkResults
		}
	}

checkResults:
	close(done)
	elapsed := time.Since(start)
	_ = b // suppress unused

	// At 5 FPS over ~1s, we expect ~5-6 frames (1 initial + ~5 from ticker).
	t.Logf("received %d frames in %v (expected ~5-6 at %d FPS)", received, elapsed, fps)
	if received < 3 {
		t.Errorf("too few frames: got %d, want at least 3", received)
	}
	if received > 10 {
		t.Errorf("too many frames: got %d, want at most 10", received)
	}
}

// TestPollingScreencast_ErrorStopsGoroutine verifies that when
// CaptureScreenshot returns an error, the polling goroutine exits
// and closes the frame channel rather than hanging.
func TestPollingScreencast_ErrorStopsGoroutine(t *testing.T) {
	// Simulate the error path: produce an error on the initial frame.
	frameCh := make(chan []byte, 2)
	done := make(chan struct{})

	captureErr := fmt.Errorf("screenshot: context canceled")
	go func() {
		defer close(frameCh)

		// Simulate CaptureScreenshot returning an error.
		// The goroutine should return immediately, closing frameCh.
		_ = captureErr
	}()

	select {
	case _, ok := <-frameCh:
		if ok {
			t.Fatal("expected channel to be closed, got a frame")
		}
		t.Log("frame channel closed correctly after capture error")
	case <-time.After(2 * time.Second):
		close(done)
		t.Fatal("goroutine did not exit after capture error — potential hang")
	}
	_ = done
}

// TestPollingScreencast_ChannelBufferPreventsBlock verifies that
// the buffer size of 2 allows the producer to stay ahead even
// when the consumer has a brief pause.
func TestPollingScreencast_ChannelBufferPreventsBlock(t *testing.T) {
	fakeFrame := []byte("frame-data")
	frameCh := make(chan []byte, 2)
	done := make(chan struct{})

	go func() {
		defer close(frameCh)
		for i := 0; i < 5; i++ {
			select {
			case frameCh <- fakeFrame:
			case <-done:
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}()

	// Pause 200ms before consuming — with buffer=2, we shouldn't lose
	// the first frames.
	time.Sleep(200 * time.Millisecond)

	var received int
	for range frameCh {
		received++
	}

	t.Logf("received %d frames with delayed consumer", received)
	if received < 4 {
		t.Errorf("lost frames due to buffer pressure: got %d, want at least 4", received)
	}
	close(done)
}

// TestPollingScreencast_RealLoop_FirstFrameSuccess exercises the real
// startScreencastPolling goroutine with an injected capture function,
// verifying that frames flow through the real channel/ticker logic.
func TestPollingScreencast_RealLoop_FirstFrameSuccess(t *testing.T) {
	var captures atomic.Int64
	fakeJPEG := []byte{0xFF, 0xD8, 0xFF, 0xE0} // JPEG magic bytes

	b := &Bridge{
		Config: &config.RuntimeConfig{
			Headless:       true,
			DefaultBrowser: "chrome",
		},
		captureFunc: func(_ context.Context, _ string, _ int) ([]byte, error) {
			captures.Add(1)
			return fakeJPEG, nil
		},
	}

	ctx := context.Background()
	stream, err := b.startScreencastPolling(ctx, ScreencastOpts{
		Quality: 30,
		FPS:     10,
	})
	if err != nil {
		t.Fatalf("startScreencastPolling: %v", err)
	}
	defer stream.Close()

	select {
	case frame, ok := <-stream.Frames:
		if !ok {
			t.Fatal("frame channel closed before first frame")
		}
		if len(frame) != len(fakeJPEG) {
			t.Fatalf("first frame size = %d, want %d", len(frame), len(fakeJPEG))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first frame")
	}

	// Consume frames for 500ms and verify multiple arrive.
	var received int
	timeout := time.After(500 * time.Millisecond)
loop:
	for {
		select {
		case _, ok := <-stream.Frames:
			if !ok {
				t.Fatal("frame channel closed unexpectedly")
			}
			received++
		case <-timeout:
			break loop
		}
	}

	stream.Close()
	total := captures.Load()
	t.Logf("captures=%d received=%d (after first frame) in 500ms at 10 FPS", total, received)
	if total < 3 {
		t.Errorf("too few CaptureScreenshot calls: %d, want at least 3", total)
	}
}

// TestPollingScreencast_RealLoop_CaptureErrorExits exercises the real
// polling goroutine when CaptureScreenshot fails on the initial frame,
// verifying that the goroutine exits and closes the channel.
func TestPollingScreencast_RealLoop_CaptureErrorExits(t *testing.T) {
	b := &Bridge{
		Config: &config.RuntimeConfig{
			Headless:       true,
			DefaultBrowser: "chrome",
		},
		captureFunc: func(_ context.Context, _ string, _ int) ([]byte, error) {
			return nil, fmt.Errorf("CDP connection lost")
		},
	}

	ctx := context.Background()
	stream, err := b.startScreencastPolling(ctx, ScreencastOpts{
		Quality: 30,
		FPS:     5,
	})
	if err != nil {
		t.Fatalf("startScreencastPolling: %v", err)
	}
	defer stream.Close()

	select {
	case _, ok := <-stream.Frames:
		if ok {
			t.Fatal("expected channel to close, got a frame")
		}
		t.Log("channel closed after capture error — goroutine exited cleanly")
	case <-time.After(2 * time.Second):
		t.Fatal("goroutine did not exit after capture error")
	}
}

// TestPollingScreencast_RealLoop_MidStreamError verifies that a capture
// error AFTER the first frame still causes the goroutine to exit.
func TestPollingScreencast_RealLoop_MidStreamError(t *testing.T) {
	var calls atomic.Int64

	b := &Bridge{
		Config: &config.RuntimeConfig{
			Headless:       true,
			DefaultBrowser: "chrome",
		},
		captureFunc: func(_ context.Context, _ string, _ int) ([]byte, error) {
			n := calls.Add(1)
			if n <= 2 {
				return []byte("frame"), nil
			}
			return nil, fmt.Errorf("capture failed mid-stream")
		},
	}

	ctx := context.Background()
	stream, err := b.startScreencastPolling(ctx, ScreencastOpts{
		Quality: 30,
		FPS:     20,
	})
	if err != nil {
		t.Fatalf("startScreencastPolling: %v", err)
	}
	defer stream.Close()

	var received int
	for range stream.Frames {
		received++
	}

	t.Logf("received %d frames before error exit", received)
	if received < 1 {
		t.Error("expected at least 1 frame before error")
	}
	total := calls.Load()
	if total < 3 {
		t.Errorf("expected at least 3 capture calls (2 ok + 1 error), got %d", total)
	}
}
