package tfexec

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os/exec"
)

type (
	InitParams struct {
		Env map[string]string
	}

	ImportParams struct {
		VarsFile string
		Env      map[string]string
		Address  string
		ID       string
	}

	PlanParams struct {
		PlanFile string
		VarsFile string
		Env      map[string]string
	}

	ApplyParams struct {
		PlanFile string
		VarsFile string
		Env      map[string]string
	}

	OutputParams struct {
		Env map[string]string
	}

	DestroyParams struct {
		PlanFile string
		Env      map[string]string
	}

	Output struct {
		Value     interface{}
		Sensitive bool
	}

	NewTerraformFunc func(workDir string) (*Terraform, error)

	Terraform struct {
		tfPath  string
		workDir string
	}
)

func LazyFromPath() NewTerraformFunc {
	var resolvedPath string
	return func(workDir string) (*Terraform, error) {
		if resolvedPath == "" {
			tfPath, err := exec.LookPath("terraform")
			if err != nil {
				return nil, err
			}
			resolvedPath = tfPath
		}
		return &Terraform{
			tfPath:  resolvedPath,
			workDir: workDir,
		}, nil
	}
}

func (t *Terraform) Path() string {
	return t.tfPath
}

func (t *Terraform) WorkDir() string {
	return t.workDir
}

func (t *Terraform) Init(ctx context.Context, params InitParams) error {
	execParams := t.terraformParams([]string{"init", "-no-color"}, params.Env)
	if _, err := terraformExec(ctx, execParams); err != nil {
		return err
	}
	return nil
}

func (t *Terraform) Import(ctx context.Context, params ImportParams) error {
	args := []string{
		"import",
		"-no-color",
		"-input=false",
		"-var-file=" + params.VarsFile,
	}

	execParams := t.terraformParams(append(args, params.Address, params.ID), params.Env)
	if _, err := terraformExec(ctx, execParams); err != nil {
		return err
	}
	return nil
}

func (t *Terraform) Plan(ctx context.Context, params PlanParams) (bool, error) {
	args := []string{
		"plan",
		"-no-color",
		"-detailed-exitcode",
		"-input=false",
		"-out=" + params.PlanFile,
		"-var-file=" + params.VarsFile,
	}

	execParams := t.terraformParams(args, params.Env)
	execParams.detailedExitCode = true
	exitCode, err := terraformExec(ctx, execParams)
	if err != nil {
		return false, err
	}

	// 0 - Succeeded, diff is empty (no changes)
	// 1 - Errored
	// 2 - Succeeded, there is a diff
	switch exitCode {
	case 0:
		return false, nil // The diff is empty
	case 2:
		return true, nil // There's a diff
	default:
		return false, fmt.Errorf("terraform plan unexpected exit code: %d", exitCode)
	}
}

func (t *Terraform) Apply(ctx context.Context, params ApplyParams) error {
	args := []string{
		"apply",
		"-auto-approve",
		"-no-color",
		"-input=false",
		"-var-file=" + params.VarsFile,
		params.PlanFile,
	}

	execParams := t.terraformParams(args, params.Env)
	if _, err := terraformExec(ctx, execParams); err != nil {
		return err
	}
	return nil
}

func (t *Terraform) Destroy(ctx context.Context, params DestroyParams) error {
	args := []string{
		"destroy",
		"-auto-approve",
		"-no-color",
		"-input=false",
		params.PlanFile,
	}

	execParams := t.terraformParams(args, params.Env)
	if _, err := terraformExec(ctx, execParams); err != nil {
		return err
	}
	return nil
}

func (t *Terraform) Output(ctx context.Context, params OutputParams) (map[string]Output, error) {
	args := []string{"output", "-no-color", "-json"}

	// Collect output to parse as JSON
	output := bytes.Buffer{}
	execParams := t.terraformParams(args, params.Env)
	execParams.stdOut = io.MultiWriter(&output, execParams.stdOut)
	if _, err := terraformExec(ctx, execParams); err != nil {
		return nil, err
	}

	var parsedJson map[string]struct {
		Sensitive bool            `json:"sensitive"`
		Type      json.RawMessage `json:"type"`
		Value     json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(output.Bytes(), &parsedJson); err != nil {
		return nil, err
	}

	mappedOutput := make(map[string]Output, len(parsedJson))
	for k, v := range parsedJson {
		mappedOutput[k] = Output{
			Value:     parseJson(v.Value),
			Sensitive: v.Sensitive,
		}
	}

	return mappedOutput, nil
}

func (t *Terraform) terraformParams(args []string, env map[string]string) terraformExecParams {
	return terraformExecParams{
		tfPath:  t.tfPath,
		workDir: t.workDir,
		args:    args,
		env:     env,
		stdErr:  log.Writer(),
		stdOut:  log.Writer(),
	}
}

func parseJson(message json.RawMessage) interface{} {
	var s string
	if err := json.Unmarshal(message, &s); err == nil {
		return s
	}

	var ss []string
	if err := json.Unmarshal(message, &ss); err == nil {
		return ss
	}

	var n int
	if err := json.Unmarshal(message, &n); err == nil {
		return n
	}

	return message
}
