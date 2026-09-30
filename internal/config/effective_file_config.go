package config

import "reflect"

// EffectiveFileConfig fills every key fc leaves unset with the built-in default, so a
// reader sees the value in effect instead of an absence it has to resolve itself.
// Host-derived paths stay as the file has them: writing the resolved home directory
// back would pin it into a file that should keep following server.stateDir. Plain
// bools are never filled, because an explicit false is indistinguishable from unset.
func EffectiveFileConfig(fc FileConfig) FileConfig {
	defaults := FileConfigFromRuntime(defaultRuntimeConfig())
	defaults.Schema = ""
	defaults.Server.StateDir = ""
	defaults.Browser.ExtensionPaths = nil
	fillUnset(reflect.ValueOf(&fc).Elem(), reflect.ValueOf(defaults))
	return fc
}

func fillUnset(dst, src reflect.Value) {
	for i := range dst.NumField() {
		field, fallback := dst.Field(i), src.Field(i)
		switch {
		case field.Kind() == reflect.Struct:
			fillUnset(field, fallback)
		case field.Kind() == reflect.Pointer && field.Elem().Kind() == reflect.Struct && !fallback.IsNil():
			filled := reflect.New(field.Elem().Type())
			filled.Elem().Set(field.Elem())
			fillUnset(filled.Elem(), fallback.Elem())
			field.Set(filled)
		case field.Kind() != reflect.Bool && field.IsZero():
			field.Set(fallback)
		}
	}
}
