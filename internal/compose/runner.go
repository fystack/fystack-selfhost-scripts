package compose

import (
	"context"
	"os/exec"
)

type Runner interface {
	Run(ctx context.Context, dir, name string, args ...string) ([]byte, error)
	LookPath(name string) error
}

type OSRunner struct{}

func (OSRunner) Run(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

func (OSRunner) LookPath(name string) error {
	_, err := exec.LookPath(name)
	return err
}
