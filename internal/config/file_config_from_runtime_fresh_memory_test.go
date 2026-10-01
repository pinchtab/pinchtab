package config

import (
	"reflect"
	"testing"
)

func scribbleReferences(v reflect.Value) int {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return 0
		}
		return 1 + scribbleValue(v.Elem())
	case reflect.Slice:
		n := 0
		for i := 0; i < v.Len(); i++ {
			n += 1 + scribbleValue(v.Index(i))
		}
		return n
	case reflect.Map:
		n := 0
		for _, key := range v.MapKeys() {
			elem := reflect.New(v.Type().Elem()).Elem()
			elem.Set(v.MapIndex(key))
			n += 1 + scribbleValue(elem)
			v.SetMapIndex(key, elem)
		}
		return n
	case reflect.Struct:
		n := 0
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() {
				n += scribbleReferences(v.Field(i))
			}
		}
		return n
	}
	return 0
}

func scribbleValue(v reflect.Value) int {
	switch v.Kind() {
	case reflect.Bool:
		v.SetBool(!v.Bool())
	case reflect.String:
		v.SetString(v.String() + "-mutated")
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(v.Int() + 1)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(v.Uint() + 1)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(v.Float() + 1)
	default:
		return scribbleReferences(v)
	}
	return 0
}

func emptyRuntimeReferences(path string, v reflect.Value) []string {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return []string{path}
		}
		return emptyRuntimeReferences(path, v.Elem())
	case reflect.Slice, reflect.Map:
		if v.Len() == 0 {
			return []string{path}
		}
	case reflect.Struct:
		var empty []string
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() {
				empty = append(empty, emptyRuntimeReferences(path+"."+v.Type().Field(i).Name, v.Field(i))...)
			}
		}
		return empty
	}
	return nil
}

func aliasProbeRuntimeConfig(t *testing.T) *RuntimeConfig {
	t.Helper()
	cfg := populatedRuntimeConfig(t)
	cfg.CookieSecure = new(true)
	cfg.AllowedDomains = []string{"allowed.example"}
	cfg.Proxy.BypassList = []string{"bypass.example"}
	cfg.Proxy.Geo = &BrowserProxyGeoConfig{Timezone: "Europe/Rome", Locale: "it-IT", WebRTCIP: "203.0.113.9", CountryISO: "IT"}
	if empty := emptyRuntimeReferences("RuntimeConfig", reflect.ValueOf(cfg).Elem()); len(empty) > 0 {
		t.Fatalf("the alias probe leaves these reference-typed runtime fields empty, so an alias through them is invisible: %v", empty)
	}
	return cfg
}

func TestWritingThroughEveryFileConfigReferenceLeavesTheRuntimeUnchanged(t *testing.T) {
	cfg := aliasProbeRuntimeConfig(t)
	want := aliasProbeRuntimeConfig(t)
	if !reflect.DeepEqual(cfg, want) {
		t.Fatal("populatedRuntimeConfig is not deterministic, so it cannot serve as the control")
	}

	fc := FileConfigFromRuntime(cfg)
	if written := scribbleReferences(reflect.ValueOf(&fc).Elem()); written < 80 {
		t.Fatalf("wrote through only %d references; the fixture no longer populates the reference-typed fields", written)
	}

	if !reflect.DeepEqual(cfg, want) {
		t.Fatalf("a write through the returned FileConfig reached the runtime:\n got %+v\nwant %+v", *cfg, *want)
	}
}
