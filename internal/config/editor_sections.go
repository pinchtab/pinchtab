package config

import (
	"fmt"
	"strings"
)

// configSection binds a settable top-level section of FileConfig to the pair of
// accessors that reach into it.
type configSection struct {
	name string
	set  func(fc *FileConfig, field, value string) error
	get  func(fc *FileConfig, field string) (string, error)
}

// configSections is the ONE place the section vocabulary is written down. It used to
// live in four: a switch in editor_set.go, a switch in editor_get.go, and a hand-typed
// valid-sections list inside the error string at each default arm. Nothing tied any of
// them to FileConfig, so adding a section to the struct was silently correct everywhere
// except here — which is how autoSolver and scheduler both came to be declared, honoured
// by the config file, documented, exposed by the dashboard, and rejected by the CLI.
//
// autoSolver mattered most: the key-gated solver warning hands an operator
// "autoSolver.external.capsolverKey" as the thing to set, and the CLI answered that
// exact path with unknown section.
//
// $schema and configVersion are deliberately absent. They are document metadata rather
// than configuration, and the schema walk in editor_sections_test.go excludes them by
// name so neither can be added to satisfy that test nor dropped as an oversight.
var configSections = []configSection{
	{
		name: "server",
		set:  func(fc *FileConfig, f, v string) error { return serverFields.setField(&fc.Server, f, v) },
		get:  func(fc *FileConfig, f string) (string, error) { return serverFields.getField(&fc.Server, f) },
	},
	{
		name: "browser",
		set:  func(fc *FileConfig, f, v string) error { return setBrowserField(&fc.Browser, f, v) },
		get:  func(fc *FileConfig, f string) (string, error) { return getBrowserField(&fc.Browser, f) },
	},
	{
		name: "browsers",
		set:  func(fc *FileConfig, f, v string) error { return setBrowsersField(&fc.Browsers, f, v) },
		get:  func(fc *FileConfig, f string) (string, error) { return getBrowsersField(&fc.Browsers, f) },
	},
	{
		name: "instanceDefaults",
		set:  func(fc *FileConfig, f, v string) error { return setInstanceDefaultsField(&fc.InstanceDefaults, f, v) },
		get: func(fc *FileConfig, f string) (string, error) {
			return getInstanceDefaultsField(&fc.InstanceDefaults, f)
		},
	},
	{
		name: "security",
		set:  func(fc *FileConfig, f, v string) error { return setSecurityField(&fc.Security, f, v) },
		get:  func(fc *FileConfig, f string) (string, error) { return getSecurityField(&fc.Security, f) },
	},
	{
		name: "profiles",
		set:  func(fc *FileConfig, f, v string) error { return setProfilesField(&fc.Profiles, f, v) },
		get:  func(fc *FileConfig, f string) (string, error) { return getProfilesField(&fc.Profiles, f) },
	},
	{
		name: "multiInstance",
		set:  func(fc *FileConfig, f, v string) error { return setMultiInstanceField(&fc.MultiInstance, f, v) },
		get:  func(fc *FileConfig, f string) (string, error) { return getMultiInstanceField(&fc.MultiInstance, f) },
	},
	{
		name: "timeouts",
		set:  func(fc *FileConfig, f, v string) error { return setTimeoutsField(&fc.Timeouts, f, v) },
		get:  func(fc *FileConfig, f string) (string, error) { return getTimeoutsField(&fc.Timeouts, f) },
	},
	{
		name: "scheduler",
		set:  func(fc *FileConfig, f, v string) error { return setSchedulerField(&fc.Scheduler, f, v) },
		get:  func(fc *FileConfig, f string) (string, error) { return getSchedulerField(&fc.Scheduler, f) },
	},
	{
		name: "observability",
		set:  func(fc *FileConfig, f, v string) error { return setObservabilityField(&fc.Observability, f, v) },
		get:  func(fc *FileConfig, f string) (string, error) { return getObservabilityField(&fc.Observability, f) },
	},
	{
		name: "sessions",
		set:  func(fc *FileConfig, f, v string) error { return setSessionsField(&fc.Sessions, f, v) },
		get:  func(fc *FileConfig, f string) (string, error) { return getSessionsField(&fc.Sessions, f) },
	},
	{
		name: "autoSolver",
		set:  func(fc *FileConfig, f, v string) error { return setAutoSolverField(&fc.AutoSolver, f, v) },
		get:  func(fc *FileConfig, f string) (string, error) { return getAutoSolverField(&fc.AutoSolver, f) },
	},
}

// lookupConfigSection splits a dotted path and returns the section that owns it.
func lookupConfigSection(path string) (configSection, string, error) {
	parts := strings.SplitN(path, ".", 2)
	if len(parts) != 2 {
		return configSection{}, "", fmt.Errorf("invalid path %q (expected section.field, e.g., server.port)", path)
	}
	name, field := parts[0], parts[1]
	for _, section := range configSections {
		if section.name == name {
			return section, field, nil
		}
	}
	return configSection{}, "", fmt.Errorf("unknown section %q (valid: %s)", name, configSectionNames())
}

// configSectionNames renders the valid sections for the refusal, derived from the table
// rather than typed out, so the message cannot describe a vocabulary the resolvers no
// longer have.
func configSectionNames() string {
	names := make([]string, 0, len(configSections))
	for _, section := range configSections {
		names = append(names, section.name)
	}
	return strings.Join(names, ", ")
}
