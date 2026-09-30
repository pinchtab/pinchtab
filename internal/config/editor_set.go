package config

import (
	"fmt"
	"strings"
)

// SetConfigValue sets a dotted path in FileConfig (e.g., "server.port", "instanceDefaults.mode").
// The section vocabulary lives once, in configSections.
func SetConfigValue(fc *FileConfig, path string, value string) error {
	section, field, err := lookupConfigSection(path)
	if err != nil {
		return err
	}
	return section.set(fc, field, value)
}

func setBrowserTargetField(targets *BrowserTargetsConfig, at, path, value string) error {
	name, field, ok := strings.Cut(path, ".")
	if !ok || name == "" || field == "" {
		return fmt.Errorf("invalid browser.targets path %q (expected targets.<name>.<field>)", path)
	}
	if !IsValidBrowserTargetName(name) {
		return fmt.Errorf("invalid browser target name %q (must match ^[a-z][a-z0-9-]{0,31}$)", name)
	}
	if *targets == nil {
		*targets = BrowserTargetsConfig{}
	}
	t := (*targets)[name]
	if err := browserTargetFields.set(&t, at+"."+name, field, value); err != nil {
		return err
	}
	(*targets)[name] = t
	return nil
}
