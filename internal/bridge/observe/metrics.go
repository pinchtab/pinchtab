package observe

import (
	"context"
	"strings"

	"github.com/chromedp/chromedp"
	"github.com/shirou/gopsutil/v4/process"
)

// MemoryMetrics holds what PinchTab measures: RSS across the browser process
// tree and the renderer count. Every field is measured, never derived from
// another field in the same payload.
type MemoryMetrics struct {
	MemoryMB  float64 `json:"memoryMB"`
	Renderers int     `json:"renderers"`
}

func newMemoryMetrics(totalBytes uint64, renderers int) *MemoryMetrics {
	return &MemoryMetrics{MemoryMB: float64(totalBytes) / (1024 * 1024), Renderers: renderers}
}

// GetAggregatedMemoryMetrics returns OS-level memory usage across the browser process tree.
func GetAggregatedMemoryMetrics(browserCtx context.Context) (*MemoryMetrics, error) {
	if browserCtx == nil {
		return nil, nil
	}

	result := &MemoryMetrics{}
	browser := chromedp.FromContext(browserCtx)
	if browser == nil || browser.Browser == nil {
		return result, nil
	}

	proc := browser.Browser.Process()
	if proc == nil {
		return result, nil
	}

	mainPID := int32(proc.Pid)
	p, err := process.NewProcess(mainPID)
	if err != nil {
		return result, err
	}

	children, err := p.Children()
	if err != nil {
		mem, _ := getProcessMemory(mainPID)
		return newMemoryMetrics(mem, 0), nil
	}

	var totalMem uint64
	rendererCount := 0

	mem, _ := getProcessMemory(mainPID)
	totalMem += mem

	for _, child := range children {
		cmdline, _ := child.Cmdline()
		if containsRenderer(cmdline) {
			rendererCount++
		}
		childMem, _ := getProcessMemory(child.Pid)
		totalMem += childMem
	}

	return newMemoryMetrics(totalMem, rendererCount), nil
}

func getProcessMemory(pid int32) (uint64, error) {
	p, err := process.NewProcess(pid)
	if err != nil {
		return 0, err
	}

	mem, err := p.MemoryInfo()
	if err != nil {
		return 0, err
	}

	return mem.RSS, nil
}

func containsRenderer(cmdline string) bool {
	return strings.Contains(cmdline, "--type=renderer") || strings.Contains(cmdline, "--type=tab")
}
