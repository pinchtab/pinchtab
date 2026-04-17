package orchestrator

import (
	"context"
	"io"
	"os/exec"
)

type PortInspection struct {
	Available bool
	PID       int
	Command   string
}

type HostRunner interface {
	Run(ctx context.Context, binary string, args []string, env []string, stdout, stderr io.Writer) (Cmd, error)
	InspectPort(port string) PortInspection
}

type Cmd interface {
	Wait() error
	PID() int
	Cancel()
}

type LocalRunner struct{}

type localCmd struct {
	execCmd *exec.Cmd
	cancel  context.CancelFunc
}

func (r *LocalRunner) Run(ctx context.Context, binary string, args []string, env []string, stdout, stderr io.Writer) (Cmd, error) {
	runCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(runCtx, binary, args...) // #nosec G204 -- binary is the pinchtab executable path, args are internal subcommands
	cmd.Env = env
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	setProcGroup(cmd)

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}

	return &localCmd{execCmd: cmd, cancel: cancel}, nil
}

func (r *LocalRunner) InspectPort(port string) PortInspection {
	return inspectPort(port)
}

func (c *localCmd) Wait() error {
	return c.execCmd.Wait()
}

func (c *localCmd) PID() int {
	if c.execCmd.Process != nil {
		return c.execCmd.Process.Pid
	}
	return 0
}

func (c *localCmd) Cancel() {
	c.cancel()
}
