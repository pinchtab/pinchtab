package orchestrator

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge/observe"
)

func jsonKeyPaths(typ reflect.Type, prefix string, into *[]string) {
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		key := strings.Split(field.Tag.Get("json"), ",")[0]
		leaf := field.Type
		if leaf.Kind() == reflect.Ptr {
			leaf = leaf.Elem()
		}
		if leaf.Kind() == reflect.Struct {
			jsonKeyPaths(leaf, prefix+key+".", into)
			continue
		}
		*into = append(*into, prefix+key)
	}
}

// The orchestrator decodes an instance's /metrics memory block into its own DTO;
// a key missing here is a measurement the front door silently drops.
func TestTheOrchestratorMemoryDTOMirrorsTheMeasuredMemoryMetricsKeyForKey(t *testing.T) {
	var measured, wire []string
	jsonKeyPaths(reflect.TypeOf(observe.MemoryMetrics{}), "", &measured)
	jsonKeyPaths(reflect.TypeOf(memoryMetrics{}), "", &wire)
	sort.Strings(measured)
	sort.Strings(wire)
	if strings.Join(measured, ",") != strings.Join(wire, ",") {
		t.Fatalf("memoryMetrics keys %v, want the measured keys %v", wire, measured)
	}
}
