package tfworkspace

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"syscall"

	"github.com/dynajoe/temporal-terraform-demo/tfexec"
)

type (
	PlanOutput struct {
		PlanFile   string
		HasChanges bool
		Summary    string
	}

	ApplyInput struct {
		Env  map[string]string
		Vars map[string]any
	}

	ApplyOutput struct {
		Output map[string]any
	}

	DestroyInput struct {
		Env  map[string]string
		Vars map[string]any
	}

	Workspace struct {
		bundlePath string
		tf         tfexec.NewTerraformFunc
	}
)

func NewFromBundle(bundlePath string) *Workspace {
	return &Workspace{bundlePath: bundlePath, tf: tfexec.LazyFromPath()}
}

func (w *Workspace) init(ctx context.Context) (tf *tfexec.Terraform, cleanup func(), retErr error) {
	// Create temporary workspace
	workDir, err := os.MkdirTemp("", "tf-")
	if err != nil {
		return nil, nil, fmt.Errorf("error creating terraform directory: %w", err)
	}
	defer func() {
		if retErr != nil {
			_ = os.RemoveAll(workDir)
		}
	}()

	// Unzip the contents of terraform bundle
	if _, err := unzip(w.bundlePath, workDir); err != nil {
		return nil, nil, err
	}

	// Get terraform executable interface
	tf, err = w.tf(workDir)
	if err != nil {
		return nil, nil, err
	}

	// terraform init
	if err := tf.Init(ctx, tfexec.InitParams{}); err != nil {
		return nil, nil, err
	}

	return tf, func() {
		_ = os.RemoveAll(workDir)
	}, nil
}

func (w *Workspace) Plan(ctx context.Context, env map[string]string) (PlanOutput, error) {
	// Init workspace
	tf, cleanup, err := w.init(ctx)
	if err != nil {
		return PlanOutput{}, err
	}
	defer cleanup()

	// Temporary file to write plan
	planFile, err := os.CreateTemp("", "terraform.*.tf-plan")
	if err != nil {
		return PlanOutput{}, err
	}

	// Terraform plan
	hasChanges, err := tf.Plan(ctx, tfexec.PlanParams{
		Env:      env,
		PlanFile: planFile.Name(),
		VarsFile: path.Join(tf.WorkDir(), "terraform.tfvars.json"),
	})
	if err != nil {
		return PlanOutput{}, fmt.Errorf("terraform plan error: %w", err)
	}

	return PlanOutput{
		PlanFile:   planFile.Name(),
		HasChanges: hasChanges,
	}, nil
}

func (w *Workspace) Destroy(ctx context.Context, env map[string]string, planFile string) error {
	// Init workspace
	tf, cleanup, err := w.init(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	if err := tf.Destroy(ctx, tfexec.DestroyParams{
		PlanFile: planFile,
		Env:      env,
	}); err != nil {
		return fmt.Errorf("terraform destroy error: %w", err)
	}

	return nil
}

func (w *Workspace) Apply(ctx context.Context, env map[string]string, planFile string) (ApplyOutput, error) {
	// Init workspace
	tf, cleanup, err := w.init(ctx)
	if err != nil {
		return ApplyOutput{}, err
	}
	defer cleanup()

	// Terraform apply plan-file
	if err := tf.Apply(ctx, tfexec.ApplyParams{
		PlanFile: planFile,
		Env:      env,
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

func unzip(src, dest string) ([]string, error) {
	if err := os.MkdirAll(dest, os.ModePerm); err != nil {
		return nil, err
	}

	r, err := zip.OpenReader(src)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	// Extract files and make dirs
	var extracted []string
	for _, f := range r.File {
		extracted = append(extracted, f.Name)
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(filepath.Join(dest, f.Name), os.ModePerm); err != nil {
				return nil, err
			}
			continue
		}

		if err := extractFile(f, dest); err != nil {
			return nil, err
		}
	}

	return extracted, nil
}

func extractFile(zfile *zip.File, destDir string) (err error) {
	fpath := filepath.Join(destDir, zfile.Name)

	if err = os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
		return err
	}

	rc, err := zfile.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	if zfile.Mode()&os.ModeSymlink != 0 {
		oldname, err := io.ReadAll(rc)
		if err != nil {
			return err
		}
		return syscall.Symlink(string(oldname), fpath)
	}

	outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, zfile.Mode())
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := outFile.Close(); err == nil {
			err = closeErr
		}
	}()

	_, err = io.Copy(outFile, rc)
	return err
}
