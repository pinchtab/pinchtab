package config

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// The standing defect class this section is exposed under: SchedulerFileConfig is the type
// `config set` writes into, and fileConfigJSON.Scheduler is a SEPARATE hand-maintained twin
// that MarshalJSON copies into. A field present on the first and missing from the second
// makes `config set` report success and write nothing.
//
// Derived from the type rather than listed, so a scheduler key added later is covered with
// no edit here: every declared field is set through the editor, marshaled the way a save
// does it, and read back out of the JSON.
func TestEverySchedulerKeySurvivesTheWireTwin(t *testing.T) {
	fields := reflect.VisibleFields(reflect.TypeFor[SchedulerFileConfig]())
	if len(fields) == 0 {
		t.Fatal("SchedulerFileConfig declares no fields; this guard would pass vacuously")
	}

	var file SchedulerFileConfig
	want := map[string]any{}
	checked := 0
	for _, field := range fields {
		key, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if key == "" || key == "-" {
			continue
		}
		value, expected, ok := schedulerProbeValueFor(field.Type)
		if !ok {
			t.Errorf("%q has type %s, which this guard cannot drive — extend it rather than skipping the field", key, field.Type)
			continue
		}
		if err := schedulerFields.set(&file, "scheduler", key, value); err != nil {
			t.Errorf("config set scheduler.%s: %v", key, err)
			continue
		}
		want[key] = expected
		checked++
	}
	if checked == 0 {
		t.Fatal("no field was driven; the guard is not exercising the editor")
	}

	raw, err := json.Marshal(FileConfig{Scheduler: file})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var onDisk struct {
		Scheduler map[string]any `json:"scheduler"`
	}
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for key, expected := range want {
		got, present := onDisk.Scheduler[key]
		if !present || got == nil {
			t.Errorf("scheduler.%s was accepted by config set and is absent from the saved file; the wire twin is missing it, so the write reports success and changes nothing", key)
			continue
		}
		if got != expected {
			t.Errorf("scheduler.%s saved as %v, want %v", key, got, expected)
		}
	}
}

// The other half: a key the file sets must reach the runtime and then the reader, so the
// value an operator writes is the value in effect rather than one the loader dropped.
func TestSchedulerMaxBatchSizeReachesTheRuntimeFromTheFile(t *testing.T) {
	writeConfigForGet(t, `{"scheduler":{"maxBatchSize":5}}`)

	cfg, _, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Scheduler.MaxBatchSize != 5 {
		t.Errorf("runtime maxBatchSize = %d, want the configured 5", cfg.Scheduler.MaxBatchSize)
	}
	if got := effectiveValue(t, "scheduler.maxBatchSize"); got != "5" {
		t.Errorf("scheduler.maxBatchSize reads %q, want the configured 5", got)
	}
}

func schedulerProbeValueFor(t reflect.Type) (string, any, bool) {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Bool:
		return "true", true, true
	case reflect.String:
		return "strict-fifo", "strict-fifo", true
	case reflect.Int:
		return "7", float64(7), true
	default:
		return "", nil, false
	}
}
