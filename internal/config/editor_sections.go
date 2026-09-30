package config

import (
	"fmt"
	"strings"
)

// configSection binds a settable top-level section of FileConfig to the pair of
// accessors that reach into it.
type configSection struct {
	name   string
	set    func(fc *FileConfig, field, value string) error
	get    func(fc *FileConfig, field string) (string, error)
	leaves func() []fieldLeaf
}

func tableSection[T any](name string, tbl *fieldTable[T], slot func(*FileConfig) *T) configSection {
	return configSection{
		name:   name,
		set:    func(fc *FileConfig, field, value string) error { return tbl.set(slot(fc), name, field, value) },
		get:    func(fc *FileConfig, field string) (string, error) { return tbl.get(slot(fc), name, field) },
		leaves: func() []fieldLeaf { return tbl.leaves(name) },
	}
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
	tableSection("server", &serverFields, func(fc *FileConfig) *ServerConfig { return &fc.Server }),
	tableSection("browser", &browserFields, func(fc *FileConfig) *BrowserConfig { return &fc.Browser }),
	tableSection("browsers", &browsersFields, func(fc *FileConfig) *BrowsersConfig { return &fc.Browsers }),
	tableSection("instanceDefaults", &instanceDefaultsFields, func(fc *FileConfig) *InstanceDefaultsConfig { return &fc.InstanceDefaults }),
	tableSection("security", &securityFields, func(fc *FileConfig) *SecurityConfig { return &fc.Security }),
	tableSection("profiles", &profilesFields, func(fc *FileConfig) *ProfilesConfig { return &fc.Profiles }),
	tableSection("multiInstance", &multiInstanceFields, func(fc *FileConfig) *MultiInstanceConfig { return &fc.MultiInstance }),
	tableSection("timeouts", &timeoutsFields, func(fc *FileConfig) *TimeoutsConfig { return &fc.Timeouts }),
	tableSection("scheduler", &schedulerFields, func(fc *FileConfig) *SchedulerFileConfig { return &fc.Scheduler }),
	tableSection("observability", &observabilityFields, func(fc *FileConfig) *ObservabilityFileConfig { return &fc.Observability }),
	tableSection("sessions", &sessionsFields, func(fc *FileConfig) *SessionsFileConfig { return &fc.Sessions }),
	tableSection("autoSolver", &autoSolverFields, func(fc *FileConfig) *AutoSolverFileConfig { return &fc.AutoSolver }),
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
