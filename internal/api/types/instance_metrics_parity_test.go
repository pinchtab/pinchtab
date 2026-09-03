package types

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

// InstanceMetrics is the hand-maintained wire twin of observe.MemoryMetrics plus the
// instance identity. A key on one side and not the other is drift: a measured field
// the dashboard never sees, or a field on the wire nothing measured.
func TestInstanceMetricsMirrorsTheMeasuredMemoryMetricsKeyForKey(t *testing.T) {
	var measured, wire []string
	jsonKeyPaths(reflect.TypeOf(observe.MemoryMetrics{}), "", &measured)
	jsonKeyPaths(reflect.TypeOf(InstanceMetrics{}), "", &wire)
	identity := map[string]bool{"instanceId": true, "profileName": true}
	var payload []string
	for _, key := range wire {
		if !identity[key] {
			payload = append(payload, key)
		}
	}
	sort.Strings(measured)
	sort.Strings(payload)
	if strings.Join(measured, ",") != strings.Join(payload, ",") {
		t.Fatalf("InstanceMetrics keys %v, want the measured keys %v beside the instance identity", payload, measured)
	}
	if len(measured) < 4 {
		t.Fatalf("only %d measured keys walked; the walk missed the nested page block", len(measured))
	}
}
