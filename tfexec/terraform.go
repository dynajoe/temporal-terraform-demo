package tfexec

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path"
	"text/template"

	"github.com/aws/aws-sdk-go-v2/aws"
)

type (
	InitParams struct {
		Backend S3BackendConfig
	}

	ImportParams struct {
		Vars    map[string]interface{}
		Env     map[string]string
		Address string
		ID      string
	}

	ApplyParams struct {
		Vars map[string]interface{}
		Env  map[string]string
	}

	OutputParams struct {
		Env map[string]string
	}

	DestroyParams struct {
		Vars map[string]interface{}
		Env  map[string]string
	}

	Output struct {
		Value     interface{}
		Sensitive bool
	}

	S3BackendConfig struct {
		Bucket      string
		Key         string
		Region      string
		Env         map[string]string
		Credentials aws.CredentialsProvider
	}

	s3BackendConfigTemplateVars struct {
		Bucket    string
		Key       string
		Region    string
		AccessKey string
		SecretKey string
		Token     string
	}

	NewTerraformFunc func(workDir string) (*Terraform, error)

	Terraform struct {
		tfPath  string
		workDir string
	}
)

var backendConfigTemplate = template.Must(template.New("terraform backend config").Parse(`
terraform {
	backend "s3" {
      encrypt    = true
	  bucket     = "{{ .Bucket }}"
	  key        = "{{ .Key }}"
	  region     = "{{ .Region }}"
	  access_key = "{{ .AccessKey }}"
	  secret_key = "{{ .SecretKey }}"
	  token      = "{{ .Token }}"
	}
}
`))

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

func (t *Terraform) Init(ctx context.Context, params InitParams) error {
	creds, err := params.Backend.Credentials.Retrieve(ctx)
	if err != nil {
		return err
	}

	// Ensure backend is configured for s3
	configBuf := bytes.Buffer{}
	if err := backendConfigTemplate.Execute(&configBuf, s3BackendConfigTemplateVars{
		Bucket:    params.Backend.Bucket,
		Key:       params.Backend.Key,
		Region:    params.Backend.Region,
		AccessKey: creds.AccessKeyID,
		SecretKey: creds.SecretAccessKey,
		Token:     creds.SessionToken,
	}); err != nil {
		return fmt.Errorf("error creating backend config: %w", err)
	}

	if err := os.WriteFile(path.Join(t.workDir, "_backend.tf"), configBuf.Bytes(), os.ModePerm); err != nil {
		return err
	}

	execParams := t.terraformParams([]string{"init", "-no-color"}, params.Backend.Env)
	if err := terraformExec(ctx, execParams); err != nil {
		return err
	}

	return nil
}

func (t *Terraform) Import(ctx context.Context, params ImportParams) error {
	args, err := t.withVars(params.Vars, []string{"import", "-no-color", "-input=false"})
	if err != nil {
		return err
	}

	execParams := t.terraformParams(append(args, params.Address, params.ID), params.Env)
	return terraformExec(ctx, execParams)
}

func (t *Terraform) Apply(ctx context.Context, params ApplyParams) error {
	args, err := t.withVars(params.Vars, []string{"apply", "-auto-approve", "-no-color", "-input=false"})
	if err != nil {
		return err
	}

	execParams := t.terraformParams(args, params.Env)
	return terraformExec(ctx, execParams)
}

func (t *Terraform) Destroy(ctx context.Context, params DestroyParams) error {
	args, err := t.withVars(params.Vars, []string{"destroy", "-auto-approve", "-no-color", "-input=false"})
	if err != nil {
		return err
	}

	execParams := t.terraformParams(args, params.Env)
	return terraformExec(ctx, execParams)
}

func (t *Terraform) Output(ctx context.Context, params OutputParams) (map[string]Output, error) {
	args := []string{"output", "-no-color", "-json"}

	// Collect output to parse as JSON
	output := bytes.Buffer{}
	execParams := t.terraformParams(args, params.Env)
	execParams.stdOut = io.MultiWriter(&output, execParams.stdOut)
	if err := terraformExec(ctx, execParams); err != nil {
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

func (t *Terraform) withVars(vars map[string]interface{}, args []string) ([]string, error) {
	if len(vars) > 0 {
		varsJson, err := json.Marshal(vars)
		if err != nil {
			return nil, err
		}

		varFilePath := path.Join(t.workDir, "terraform.tfvars.json")
		if err := os.WriteFile(varFilePath, varsJson, os.ModePerm); err != nil {
			return nil, err
		}

		args = append(args, "-var-file="+varFilePath)
	}
	return args, nil
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
