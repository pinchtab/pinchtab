package observe

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"testing"
)

func flattenNumbers(t *testing.T, prefix string, value any, into map[string]float64) {
	t.Helper()
	switch v := value.(type) {
	case float64:
		into[prefix] = v
	case map[string]any:
		for name, child := range v {
			flattenNumbers(t, prefix+"."+name, child, into)
		}
	default:
		t.Fatalf("field %q is %T, not a number or an object; this walk would silently skip it", prefix, value)
	}
}

func leafFieldCount(typ reflect.Type) int {
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return 1
	}
	count := 0
	for i := 0; i < typ.NumField(); i++ {
		count += leafFieldCount(typ.Field(i).Type)
	}
	return count
}

func numericFields(t *testing.T, m *MemoryMetrics) map[string]float64 {
	t.Helper()

	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal MemoryMetrics: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal MemoryMetrics: %v", err)
	}

	values := map[string]float64{}
	flattenNumbers(t, "", decoded, values)
	if m.Page != nil && len(values) < leafFieldCount(reflect.TypeOf(MemoryMetrics{})) {
		t.Fatalf("walked %d serialized fields for %d struct fields; a field this guard cannot see is a field it cannot police",
			len(values), leafFieldCount(reflect.TypeOf(MemoryMetrics{})))
	}
	return values
}

func sample(totalBytes uint64, renderers int, unreadable int, readings ...*PageMetrics) *MemoryMetrics {
	m := newMemoryMetrics(totalBytes, renderers)
	m.Page = sumPageMetrics(readings)
	m.UnreadableTargets = unreadable
	return m
}

// The property the derived jsHeap fields violated: nothing a caller reads may be
// arithmetic on something else in the same payload. Two samples are the minimum
// that can say so — any single pair of numbers has a ratio. The walk covers the
// nested page block, so a derived counter added there reds like a top-level one.
func TestNoReportedFieldHoldsAConstantRatioToAnotherAcrossTwoSamples(t *testing.T) {
	const mb = 1024 * 1024
	low := numericFields(t, sample(100*mb, 3, 1,
		&PageMetrics{Targets: 1, JSHeapUsedMB: 7, JSHeapTotalMB: 11, Documents: 2, Frames: 3, Nodes: 400, JSEventListeners: 5}))
	high := numericFields(t, sample(250*mb, 4, 4,
		&PageMetrics{Targets: 1, JSHeapUsedMB: 19, JSHeapTotalMB: 23, Documents: 5, Frames: 7, Nodes: 9000, JSEventListeners: 13},
		&PageMetrics{Targets: 1, JSHeapUsedMB: 29, JSHeapTotalMB: 31, Documents: 1, Frames: 1, Nodes: 37, JSEventListeners: 41}))

	for name := range low {
		for other := range low {
			if name == other || low[other] == 0 || high[other] == 0 {
				continue
			}
			lowRatio := low[name] / low[other]
			highRatio := high[name] / high[other]
			if math.Abs(lowRatio-highRatio) < 1e-9 {
				t.Errorf("%s/%s is %v in both samples; %s is derived from %s rather than measured, and a gauge built on the pair cannot move",
					name, other, lowRatio, name, other)
			}
		}
	}
}

func TestMemoryMetricsReportsMeasuredRSSAndRenderers(t *testing.T) {
	const mb = 1024 * 1024
	got := numericFields(t, newMemoryMetrics(250*mb, 4))

	want := map[string]float64{".memoryMB": 250, ".renderers": 4, ".unreadableTargets": 0}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("MemoryMetrics = %v, want %v", got, want)
	}
}

// Absent and could-not-read never share a representation: no targets is an absent
// page block with nothing unreadable; targets that would not answer is the same
// absent block with the refusals counted; a partial read sums only the answers.
func TestAnEmptyReadingAndAFailedReadAreDistinguishableInThePayload(t *testing.T) {
	encode := func(m *MemoryMetrics) map[string]any {
		raw, _ := json.Marshal(m)
		var out map[string]any
		_ = json.Unmarshal(raw, &out)
		return out
	}

	empty := encode(sample(1, 1, 0))
	if _, present := empty["page"]; present || empty["unreadableTargets"] != float64(0) {
		t.Errorf("no targets = %v, want no page block and nothing unreadable", empty)
	}

	failed := encode(sample(1, 1, 2))
	if _, present := failed["page"]; present || failed["unreadableTargets"] != float64(2) {
		t.Errorf("all targets failed = %v, want no page block and two unreadable", failed)
	}

	partial := sample(1, 1, 1, &PageMetrics{Targets: 1, Nodes: 10, JSHeapUsedMB: 1}, &PageMetrics{Targets: 1, Nodes: 5, JSHeapUsedMB: 2})
	if partial.Page == nil || partial.Page.Targets != 2 || partial.Page.Nodes != 15 || partial.Page.JSHeapUsedMB != 3 || partial.UnreadableTargets != 1 {
		t.Errorf("partial = %+v / unreadable %d, want the two answers summed and one refusal counted", partial.Page, partial.UnreadableTargets)
	}
}

func TestReadTargetsCountsATargetThatWouldNotAnswerWithoutContributingZeros(t *testing.T) {
	page, unreadable := readTargets(map[string]context.Context{"dead": cancelledContext()})
	if page != nil || unreadable != 1 {
		t.Errorf("readTargets over a dead target = %+v / %d, want no page block and one unreadable", page, unreadable)
	}
	if page, unreadable := readTargets(nil); page != nil || unreadable != 0 {
		t.Errorf("readTargets over nothing = %+v / %d, want absent and nothing unreadable", page, unreadable)
	}
}

func cancelledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}
