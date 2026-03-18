package daemon

import (
	"strings"
)

type fakeCommandRunner struct {
	calls   []string
	outputs map[string]string
	errors  map[string]error
}

func (f *fakeCommandRunner) CombinedOutput(name string, args ...string) ([]byte, error) {
	call := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, call)
	if out, ok := f.outputs[call]; ok {
		return []byte(out), f.errors[call]
	}
	return nil, f.errors[call]
}
