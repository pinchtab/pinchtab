package observe

import (
	"context"
	"strings"
	"time"

	"github.com/chromedp/cdproto/performance"
	"github.com/chromedp/chromedp"
	"github.com/shirou/gopsutil/v4/process"
)

const targetReadTimeout = 2 * time.Second

// MemoryMetrics holds what PinchTab measures for one browser instance: RSS across
// the process tree, the renderer count, and the page counters summed over every
// target that answered. Every field is measured, never derived from another field
// in the same payload. Page is absent when no target contributed; a target that
// could not be read is counted in UnreadableTargets and contributes nothing, so
// "no tabs" and "tabs that would not answer" never share a representation.
type MemoryMetrics struct {
	MemoryMB          float64      `json:"memoryMB"`
	Renderers         int          `json:"renderers"`
	Page              *PageMetrics `json:"page,omitempty"`
	UnreadableTargets int          `json:"unreadableTargets"`
}

// PageMetrics is Performance.getMetrics summed over Targets targets.
type PageMetrics struct {
	Targets          int     `json:"targets"`
	JSHeapUsedMB     float64 `json:"jsHeapUsedMB"`
	JSHeapTotalMB    float64 `json:"jsHeapTotalMB"`
	Documents        int     `json:"documents"`
	Frames           int     `json:"frames"`
	Nodes            int     `json:"nodes"`
	JSEventListeners int     `json:"jsEventListeners"`
}

func newMemoryMetrics(totalBytes uint64, renderers int) *MemoryMetrics {
	return &MemoryMetrics{MemoryMB: float64(totalBytes) / (1024 * 1024), Renderers: renderers}
}

// ReadTargetMetrics reads one target's own Performance.getMetrics.
func ReadTargetMetrics(ctx context.Context) (*PageMetrics, error) {
	var metrics []*performance.Metric
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		if err := performance.Enable().Do(ctx); err != nil {
			return err
		}
		var err error
		metrics, err = performance.GetMetrics().Do(ctx)
		return err
	}))
	if err != nil {
		return nil, err
	}
	reading := &PageMetrics{Targets: 1}
	for _, m := range metrics {
		switch m.Name {
		case "JSHeapUsedSize":
			reading.JSHeapUsedMB = m.Value / (1024 * 1024)
		case "JSHeapTotalSize":
			reading.JSHeapTotalMB = m.Value / (1024 * 1024)
		case "Documents":
			reading.Documents = int(m.Value)
		case "Frames":
			reading.Frames = int(m.Value)
		case "Nodes":
			reading.Nodes = int(m.Value)
		case "JSEventListeners":
			reading.JSEventListeners = int(m.Value)
		}
	}
	return reading, nil
}

func sumPageMetrics(readings []*PageMetrics) *PageMetrics {
	if len(readings) == 0 {
		return nil
	}
	sum := &PageMetrics{}
	for _, r := range readings {
		sum.Targets += r.Targets
		sum.JSHeapUsedMB += r.JSHeapUsedMB
		sum.JSHeapTotalMB += r.JSHeapTotalMB
		sum.Documents += r.Documents
		sum.Frames += r.Frames
		sum.Nodes += r.Nodes
		sum.JSEventListeners += r.JSEventListeners
	}
	return sum
}

func readTargets(targets map[string]context.Context) (*PageMetrics, int) {
	readings := make([]*PageMetrics, 0, len(targets))
	unreadable := 0
	for _, targetCtx := range targets {
		ctx, cancel := context.WithTimeout(targetCtx, targetReadTimeout)
		reading, err := ReadTargetMetrics(ctx)
		cancel()
		if err != nil {
			unreadable++
			continue
		}
		readings = append(readings, reading)
	}
	return sumPageMetrics(readings), unreadable
}

// GetAggregatedMemoryMetrics returns OS-level memory usage across the browser
// process tree plus the page counters summed over the given targets.
func GetAggregatedMemoryMetrics(browserCtx context.Context, targets map[string]context.Context) (*MemoryMetrics, error) {
	if browserCtx == nil {
		return nil, nil
	}
	result, err := processTreeMemory(browserCtx)
	if err != nil {
		return result, err
	}
	result.Page, result.UnreadableTargets = readTargets(targets)
	return result, nil
}

func processTreeMemory(browserCtx context.Context) (*MemoryMetrics, error) {
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
