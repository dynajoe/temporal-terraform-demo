package tfexec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

type terraformExecParams struct {
	tfPath  string
	args    []string
	env     map[string]string
	stdErr  io.Writer
	stdOut  io.Writer
	workDir string
}

type terraformErrorInterceptor struct {
	errors []string
}

func (t *terraformErrorInterceptor) Write(p []byte) (n int, err error) {
	s := string(p)

	// Terraform prints errors like this:
	// Error: error creating EKS Node Group (dev04:app): ResourceInUseException: ...
	if strings.HasPrefix(strings.TrimSpace(s), "Error:") {
		t.errors = append(t.errors, s)
	}
	return len(p), nil
}

func terraformExec(ctx context.Context, run terraformExecParams) error {
	exited := false
	defer func() {
		exited = true
	}()

	var cmdEnv []string
	for k, v := range run.env {
		cmdEnv = append(cmdEnv, fmt.Sprintf("%s=%s", k, v))
	}

	errorInterceptor := &terraformErrorInterceptor{}

	cmd := exec.Command(run.tfPath, run.args...)
	cmd.Env = cmdEnv
	cmd.Dir = run.workDir
	cmd.SysProcAttr = osSpecificSysProcAttr()

	cmd.Stdout = io.MultiWriter(run.stdOut, errorInterceptor)
	cmd.Stderr = io.MultiWriter(run.stdErr, errorInterceptor)

	// Check context before starting
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Run the command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("terraform start command error: %s\n%w", strings.Join(errorInterceptor.errors, "\n"), err)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// cmd.Start ensures that cmd.Process is non nil
	go func() {
		// Wait for context to be canceled
		<-ctx.Done()

		// If the process has already exited no need to try to kill it
		if exited {
			return
		}

		// Send sigint to the process gorup and wait for some time to allow for graceful shutdown
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGINT); err != nil {
			if errors.Is(os.ErrProcessDone, err) {
				return
			}

			// If there was an error sending sigint just send kill
			// Using -pid will send the kill signal to process group
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			_ = cmd.Process.Kill()
		}

		// Check frequently until the process has exited
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			<-time.After(200 * time.Millisecond)
			if exited {
				return
			}
		}

		// The process hasn't exited, try to kill it again and abandon ship
		// Using -pid will send the kill signal to process group
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Process.Kill()
	}()

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("terraform error: %s\n%w", strings.Join(errorInterceptor.errors, "\n"), err)
	}
	return nil
}
