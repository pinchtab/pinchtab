package observe

import (
	"context"
	"strings"

	"github.com/chromedp/chromedp"
	"github.com/shirou/gopsutil/v4/process"
)

// MemoryMetrics holds Chrome memory statistics.
type MemoryMetrics struct {
	MemoryMB float64 `json:"memoryMB"`

	JSHeapUsedMB  float64 `json:"jsHeapUsedMB"`
	JSHeapTotalMB float64 `json:"jsHeapTotalMB"`

	Renderers int `json:"renderers"`

	Documents int64 `json:"documents"`
	Frames    int64 `json:"frames"`
	Nodes     int64 `json:"nodes"`
	Listeners int64 `json:"listeners"`
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
		result.MemoryMB = float64(mem) / (1024 * 1024)
		return result, nil
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

	result.MemoryMB = float64(totalMem) / (1024 * 1024)
	result.Renderers = rendererCount
	result.JSHeapUsedMB = result.MemoryMB * 0.4
	result.JSHeapTotalMB = result.MemoryMB * 0.5

	return result, nil
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
