package types_test

import (
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/api/types"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/bridge/observe"
	"github.com/pinchtab/pinchtab/internal/browserops"
)

var timeType = reflect.TypeFor[time.Time]()

// jsonKeys maps every JSON key path a type serialises to whether it is omitempty,
// descending into nested structs, pointers to structs and slices of structs.
func jsonKeys(typ reflect.Type) map[string]bool {
	keys := map[string]bool{}
	collectJSONKeys(typ, "", keys)
	return keys
}

func collectJSONKeys(typ reflect.Type, prefix string, into map[string]bool) {
	for typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}
		parts := strings.Split(field.Tag.Get("json"), ",")
		key := parts[0]
		if key == "-" {
			continue
		}
		if key == "" && field.Anonymous {
			if nested, ok := nestedStruct(field.Type); ok {
				collectJSONKeys(nested, prefix, into)
				continue
			}
		}
		if key == "" {
			key = field.Name
		}
		into[prefix+key] = strings.Contains(field.Tag.Get("json"), ",omitempty")
		if nested, ok := nestedStruct(field.Type); ok {
			collectJSONKeys(nested, prefix+key+".", into)
		}
	}
}

func nestedStruct(typ reflect.Type) (reflect.Type, bool) {
	for typ.Kind() == reflect.Ptr || typ.Kind() == reflect.Slice {
		typ = typ.Elem()
	}
	return typ, typ.Kind() == reflect.Struct && typ != timeType
}

type wireTwin struct {
	name   string
	wire   any
	source any
	// wireOnly lists keys the twin carries that the source type does not, each
	// because the served response adds it on top of the source.
	wireOnly map[string]string
}

var wireTwins = []wireTwin{
	{name: "Instance", wire: types.Instance{}, source: bridge.Instance{}},
	{name: "SecurityPolicy", wire: types.SecurityPolicy{}, source: bridge.SecurityPolicy{}},
	{name: "InstanceTab", wire: types.InstanceTab{}, source: bridge.InstanceTab{}},
	{name: "CrashEvent", wire: types.CrashEvent{}, source: bridge.CrashEvent{}},
	{name: "CrashSummary", wire: types.CrashSummary{}, source: bridge.CrashSummary{}},
	{name: "RouteAttempt", wire: types.RouteAttempt{}, source: browserops.RouteAttempt{}},
	{name: "RouteMetadata", wire: types.RouteMetadata{}, source: browserops.RouteMetadata{}},
	// The served profile is profiles.profileResponse, which is unexported and sends
	// every key unconditionally; Profile mirrors ProfileInfo's optionality instead,
	// which the dashboard reads the same way since an optional key may be present.
	{name: "Profile", wire: types.Profile{}, source: bridge.ProfileInfo{}, wireOnly: map[string]string{
		"sizeMB": "profiles.profileResponse derives it from diskUsage",
	}},
}

func TestEveryWireTwinMatchesItsSourceKeyForKey(t *testing.T) {
	if len(wireTwins) < 6 {
		t.Fatalf("only %d twin pairs; the table no longer covers the hand-copied types", len(wireTwins))
	}
	for _, twin := range wireTwins {
		t.Run(twin.name, func(t *testing.T) {
			wire := jsonKeys(reflect.TypeOf(twin.wire))
			source := jsonKeys(reflect.TypeOf(twin.source))
			for key, omit := range source {
				wireOmit, ok := wire[key]
				if !ok {
					t.Errorf("source key %q has no twin in types.%s", key, twin.name)
					continue
				}
				if wireOmit != omit {
					t.Errorf("key %q: types.%s omitempty = %v, source omitempty = %v", key, twin.name, wireOmit, omit)
				}
			}
			for key := range wire {
				if _, ok := source[key]; ok {
					continue
				}
				if _, exempt := twin.wireOnly[key]; !exempt {
					t.Errorf("types.%s key %q has no source field", twin.name, key)
				}
			}
			for key := range twin.wireOnly {
				if _, ok := wire[key]; !ok {
					t.Errorf("exemption %q names a key types.%s no longer has; drop it", key, twin.name)
				}
			}
		})
	}
}

func TestTheKeyWalkSeesNestedAndAddedFields(t *testing.T) {
	keys := jsonKeys(reflect.TypeFor[types.Instance]())
	for _, want := range []string{"crashes.recent.reason", "securityPolicy.allowedDomains", "startTime"} {
		if _, ok := keys[want]; !ok {
			t.Errorf("walk of types.Instance missed %q (got %v)", want, keys)
		}
	}
	type instanceWithNewField struct {
		bridge.Instance
		Added string `json:"added"`
	}
	grown := jsonKeys(reflect.TypeFor[instanceWithNewField]())
	if _, ok := grown["added"]; !ok {
		t.Fatalf("a field added beside bridge.Instance is invisible to the walk: %v", grown)
	}
	if _, ok := grown["id"]; !ok {
		t.Fatalf("an embedded bridge.Instance must flatten into its parent as encoding/json does: %v", grown)
	}
}

// InstanceMetrics is the hand-maintained wire twin of observe.MemoryMetrics plus the
// instance identity. A key on one side and not the other is drift: a measured field
// the dashboard never sees, or a field on the wire nothing measured.
func TestInstanceMetricsMirrorsTheMeasuredMemoryMetricsKeyForKey(t *testing.T) {
	identity := map[string]bool{"instanceId": true, "profileName": true}
	var measured, payload []string
	for key := range jsonKeys(reflect.TypeFor[observe.MemoryMetrics]()) {
		measured = append(measured, key)
	}
	for key := range jsonKeys(reflect.TypeFor[types.InstanceMetrics]()) {
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
