package observe

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"testing"
)

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
	for name, value := range decoded {
		number, ok := value.(float64)
		if !ok {
			t.Fatalf("field %q is %T, not a number; this walk would silently skip it", name, value)
		}
		values[name] = number
	}
	if len(values) < reflect.TypeOf(MemoryMetrics{}).NumField() {
		t.Fatalf("walked %d serialized fields for %d struct fields; a field this guard cannot see is a field it cannot police",
			len(values), reflect.TypeOf(MemoryMetrics{}).NumField())
	}
	return values
}

// The property the derived jsHeap fields violated: nothing a caller reads may be
// arithmetic on something else in the same payload. Two samples are the minimum
// that can say so — any single pair of numbers has a ratio.
func TestNoReportedFieldHoldsAConstantRatioToAnotherAcrossTwoSamples(t *testing.T) {
	const mb = 1024 * 1024
	low := numericFields(t, newMemoryMetrics(100*mb, 3))
	high := numericFields(t, newMemoryMetrics(250*mb, 4))

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

	want := map[string]float64{"memoryMB": 250, "renderers": 4}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("MemoryMetrics = %v, want %v", got, want)
	}
}
