package tfworkspace

import (
	"context"
	"embed"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/dynajoe/temporal-terraform-demo/tfexec"
)

type (
	Config struct {
		TerraformPath string
		TerraformFS   embed.FS
		S3Backend     tfexec.S3BackendConfig
	}

	ApplyInput struct {
		Env            map[string]string
		Vars           map[string]interface{}
		AttemptImport  map[string]string
		AwsCredentials aws.CredentialsProvider
	}

	ApplyOutput struct {
		Output map[string]interface{}
	}

	DestroyInput struct {
		Env            map[string]string
		Vars           map[string]interface{}
		AwsCredentials aws.CredentialsProvider
	}

	Workspace struct {
		config Config
		tf     tfexec.NewTerraformFunc
	}
)

func New(config Config) *Workspace {
	return &Workspace{config: config, tf: tfexec.LazyFromPath()}
}

func (w *Workspace) Apply(ctx context.Context, input ApplyInput) (ApplyOutput, error) {
	// Create temporary workspace
	workDir, err := ioutil.TempDir("", "tf-apply-")
	if err != nil {
		return ApplyOutput{}, fmt.Errorf("error creating terraform workspace: %w", err)
	}
	defer os.RemoveAll(workDir)

	// Extract embedded terraform to the workspace
	if err = extractEmbeddedTerraform(w.config.TerraformFS, w.config.TerraformPath, workDir); err != nil {
		return ApplyOutput{}, fmt.Errorf("error extracting terraform: %w", err)
	}

	log.Printf("initializing terraform in directory: %s", workDir)

	// Initialize terraform workspace
	tf, err := w.init(ctx, workDir)
	if err != nil {
		return ApplyOutput{}, err
	}

	// Copy env to a new map
	env := make(map[string]string, len(input.Env))
	for k, v := range input.Env {
		env[k] = v
	}

	// Add AWS creds to environment
	if input.AwsCredentials != nil {
		creds, err := input.AwsCredentials.Retrieve(ctx)
		if err != nil {
			return ApplyOutput{}, err
		}
		env["AWS_ACCESS_KEY_ID"] = creds.AccessKeyID
		env["AWS_SECRET_ACCESS_KEY"] = creds.SecretAccessKey
		env["AWS_SESSION_TOKEN"] = creds.SessionToken
	}

	// Attempt to import resources that may have not had state pushed on failure
	for k, v := range input.AttemptImport {
		// Intentionally ignoring error
		_ = tf.Import(ctx, tfexec.ImportParams{
			Env:     env,
			Vars:    input.Vars,
			Address: k,
			ID:      v,
		})

		// Check for context cancel
		if ctx.Err() != nil {
			return ApplyOutput{}, ctx.Err()
		}
	}

	if err := tf.Apply(ctx, tfexec.ApplyParams{
		Vars: input.Vars,
		Env:  env,
	}); err != nil {
		return ApplyOutput{}, fmt.Errorf("terraform apply error: %w", err)
	}

	// Extract output from successful Terraform Apply
	tfOutput, err := tf.Output(ctx, tfexec.OutputParams{
		Env: env,
	})
	if err != nil {
		return ApplyOutput{}, fmt.Errorf("terraform output error: %w", err)
	}

	output := make(map[string]interface{}, len(tfOutput))
	for k, v := range tfOutput {
		output[k] = v.Value
	}

	return ApplyOutput{
		Output: output,
	}, nil
}

func (w *Workspace) Destroy(ctx context.Context, input DestroyInput) error {
	// Create temporary workspace
	workDir, err := ioutil.TempDir("", "tf-destroy-")
	if err != nil {
		return fmt.Errorf("error creating terraform workspace: %w", err)
	}
	defer os.RemoveAll(workDir)

	// Only extract versions.tf for destroy because it's needed to determine
	// the versions of terraform providers. Every terraform directory should
	// have a versions.tf at the top level.
	versionsFileData, err := w.config.TerraformFS.ReadFile(path.Join(w.config.TerraformPath, "versions.tf"))
	if err != nil {
		return err
	}

	// Write the contents of the versions file to the workspace
	if err := os.WriteFile(path.Join(workDir, "versions.tf"), versionsFileData, 0644); err != nil {
		return err
	}

	// Initialize terraform workspace
	tf, err := w.init(ctx, workDir)
	if err != nil {
		return err
	}

	// Copy env to a new map
	env := make(map[string]string, len(input.Env))
	for k, v := range input.Env {
		env[k] = v
	}

	// Add AWS creds to environment
	if input.AwsCredentials != nil {
		creds, err := input.AwsCredentials.Retrieve(ctx)
		if err != nil {
			return err
		}
		env["AWS_ACCESS_KEY_ID"] = creds.AccessKeyID
		env["AWS_SECRET_ACCESS_KEY"] = creds.SecretAccessKey
		env["AWS_SESSION_TOKEN"] = creds.SessionToken
	}

	if err := tf.Destroy(ctx, tfexec.DestroyParams{
		Vars: input.Vars,
		Env:  env,
	}); err != nil {
		return fmt.Errorf("terraform destroy error: %w", err)
	}

	return nil
}

func (w *Workspace) init(ctx context.Context, workDir string) (*tfexec.Terraform, error) {
	tf, err := w.tf(workDir)
	if err != nil {
		return nil, err
	}

	initParams := tfexec.InitParams{
		Backend: w.config.S3Backend,
	}
	err = tf.Init(ctx, initParams)
	if err != nil {
		return nil, err
	}

	return tf, nil
}

func (o ApplyOutput) String(key string) (string, error) {
	v, ok := o.Output[key]
	if !ok {
		return "", fmt.Errorf("missing key [%s] in output", key)
	}

	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("output [%s] is not a string", key)
	}

	return s, nil
}
