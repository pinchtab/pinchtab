package config

import (
	"fmt"
	"reflect"
	"testing"
)

// CloneRuntimeConfig is a hand-written, field-by-field copy, and config.Live's
// contract — the object readers already hold is never written to — rests on it
// being complete. A reference-typed field added to RuntimeConfig and forgotten
// there is aliased into every value a save publishes, with nothing to say so:
// the compiler is happy, the round-trip tests are happy, and the save path stays
// safe only for as long as every writer of that field happens to replace the
// slice rather than write through it.
//
// So walk the type rather than listing the fields, and a field added tomorrow is
// covered the day it is added.
func TestCloneRuntimeConfigSharesNoReferenceWithItsSource(t *testing.T) {
	orig := &RuntimeConfig{}
	refs := fillReferences(reflect.ValueOf(orig).Elem())

	// A walk that reached almost nothing would pass while checking nothing. The
	// floor is well under the count today and only has to prove the walk works.
	if refs < 10 {
		t.Fatalf("filled only %d reference fields; the walk matched almost nothing and this rule would pass vacuously", refs)
	}

	clone := CloneRuntimeConfig(orig)
	if clone == nil {
		t.Fatal("CloneRuntimeConfig returned nil for a non-nil config")
	}

	var shared []string
	findShared(reflect.ValueOf(orig).Elem(), reflect.ValueOf(clone).Elem(), "RuntimeConfig", &shared)
	for _, path := range shared {
		t.Errorf("%s is shared with the source, so writing through it in a published config mutates one readers already hold; add it to CloneRuntimeConfig", path)
	}
}

// fillReferences gives every reachable slice, map and pointer a distinct
// allocation so the comparison below has something to tell apart, and reports
// how many it made.
func fillReferences(v reflect.Value) int {
	if !v.CanSet() {
		return 0
	}
	switch v.Kind() {
	case reflect.Slice:
		v.Set(reflect.MakeSlice(v.Type(), 1, 1))
		return 1 + fillReferences(v.Index(0))
	case reflect.Map:
		v.Set(reflect.MakeMap(v.Type()))
		key := reflect.New(v.Type().Key()).Elem()
		fillReferences(key)
		val := reflect.New(v.Type().Elem()).Elem()
		filled := fillReferences(val)
		v.SetMapIndex(key, val)
		return 1 + filled
	case reflect.Pointer:
		v.Set(reflect.New(v.Type().Elem()))
		return 1 + fillReferences(v.Elem())
	case reflect.Struct:
		filled := 0
		for i := range v.NumField() {
			filled += fillReferences(v.Field(i))
		}
		return filled
	case reflect.String:
		v.SetString("x")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(1)
	}
	return 0
}

// findShared records every path where the clone points at the same allocation as
// its source.
func findShared(orig, clone reflect.Value, path string, out *[]string) {
	if orig.Kind() != clone.Kind() {
		return
	}
	switch orig.Kind() {
	case reflect.Slice:
		if orig.Len() == 0 || clone.Len() == 0 {
			return
		}
		if orig.Pointer() == clone.Pointer() {
			*out = append(*out, path)
			return
		}
		for i := range min(orig.Len(), clone.Len()) {
			findShared(orig.Index(i), clone.Index(i), fmt.Sprintf("%s[%d]", path, i), out)
		}
	case reflect.Map:
		if orig.IsNil() || clone.IsNil() {
			return
		}
		if orig.Pointer() == clone.Pointer() {
			*out = append(*out, path)
			return
		}
		for _, key := range orig.MapKeys() {
			cloned := clone.MapIndex(key)
			if !cloned.IsValid() {
				continue
			}
			findShared(orig.MapIndex(key), cloned, fmt.Sprintf("%s[%v]", path, key), out)
		}
	case reflect.Pointer:
		if orig.IsNil() || clone.IsNil() {
			return
		}
		if orig.Pointer() == clone.Pointer() {
			*out = append(*out, path)
			return
		}
		findShared(orig.Elem(), clone.Elem(), path+".*", out)
	case reflect.Struct:
		for i := range orig.NumField() {
			findShared(orig.Field(i), clone.Field(i), path+"."+orig.Type().Field(i).Name, out)
		}
	}
}
