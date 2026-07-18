package observe

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestToTimingMetricsMapsFields(t *testing.T) {
	payload := `{
		"ttfb": 12.5,
		"fcp": 240.2,
		"lcp": 850.7,
		"cls": 0.031,
		"domContentLoaded": 310.1,
		"load": 905.9,
		"resourceCount": 7,
		"transferSize": 20480
	}`
	var raw rawTiming
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	m := toTimingMetrics(raw)
	if m.TTFBMs != 12.5 {
		t.Errorf("TTFBMs = %v, want 12.5", m.TTFBMs)
	}
	if m.FCPMs != 240.2 {
		t.Errorf("FCPMs = %v, want 240.2", m.FCPMs)
	}
	if m.LCPMs != 850.7 {
		t.Errorf("LCPMs = %v, want 850.7", m.LCPMs)
	}
	if m.CLS != 0.031 {
		t.Errorf("CLS = %v, want 0.031", m.CLS)
	}
	if m.DOMContentLoadedMs != 310.1 {
		t.Errorf("DOMContentLoadedMs = %v, want 310.1", m.DOMContentLoadedMs)
	}
	if m.LoadMs != 905.9 {
		t.Errorf("LoadMs = %v, want 905.9", m.LoadMs)
	}
	if m.ResourceCount != 7 {
		t.Errorf("ResourceCount = %d, want 7", m.ResourceCount)
	}
	if m.TransferSizeBytes != 20480 {
		t.Errorf("TransferSizeBytes = %d, want 20480", m.TransferSizeBytes)
	}
}

func TestToTimingMetricsClampsNegatives(t *testing.T) {
	m := toTimingMetrics(rawTiming{TTFB: -1, FCP: -2, LCP: -3, CLS: -0.5, DOMContentLoaded: -4, Load: -5, ResourceCount: -6, TransferSize: -7})
	if m.TTFBMs != 0 || m.FCPMs != 0 || m.LCPMs != 0 || m.CLS != 0 || m.DOMContentLoadedMs != 0 || m.LoadMs != 0 {
		t.Errorf("negative durations not clamped: %+v", m)
	}
	if m.ResourceCount != 0 || m.TransferSizeBytes != 0 {
		t.Errorf("negative counts not clamped: %+v", m)
	}
}

func TestToTimingMetricsZeroValue(t *testing.T) {
	m := toTimingMetrics(rawTiming{})
	if *m != (TimingMetrics{}) {
		t.Errorf("zero raw payload should map to zero metrics, got %+v", m)
	}
}

func TestTimingMetricsJSONFieldNames(t *testing.T) {
	data, err := json.Marshal(TimingMetrics{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, name := range []string{"ttfbMs", "fcpMs", "lcpMs", "cls", "domContentLoadedMs", "loadMs", "resourceCount", "transferSizeBytes"} {
		if _, ok := fields[name]; !ok {
			t.Errorf("missing JSON field %q in %s", name, data)
		}
	}
}

func TestCollectTiming(t *testing.T) {
	m, err := CollectTiming(func(expression string, result any) error {
		if expression != TimingScript {
			t.Errorf("unexpected expression evaluated")
		}
		return json.Unmarshal([]byte(`{"ttfb":3.2,"load":120.5,"resourceCount":2}`), result)
	})
	if err != nil {
		t.Fatalf("CollectTiming: %v", err)
	}
	if m.TTFBMs != 3.2 || m.LoadMs != 120.5 || m.ResourceCount != 2 {
		t.Errorf("unexpected metrics: %+v", m)
	}

	wantErr := errors.New("eval failed")
	if _, err := CollectTiming(func(string, any) error { return wantErr }); !errors.Is(err, wantErr) {
		t.Errorf("CollectTiming error = %v, want %v", err, wantErr)
	}
}
