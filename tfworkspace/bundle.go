package tfworkspace

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"text/template"
)

type BundleBuilder struct {
	metadata        map[string]string
	fsys            fs.ReadFileFS
	root            string
	vars            map[string]any
	additionalFiles map[string][]byte
	backendFunc     func() ([]byte, error)
}

type S3BackendConfig struct {
	Bucket        string
	Key           string
	Region        string
	AssumeRoleArn string
	Profile       string
}

var s3BackendTemplate = template.Must(template.New("terraform backend config").Parse(`
terraform {
	backend "s3" {
		encrypt    = true
		bucket     = "{{ .Bucket }}"
		key        = "{{ .Key }}"
		region     = "{{ .Region }}"
{{- with .Profile }}
		profile    = "{{ . }}"
{{- end }}
{{- with .AssumeRoleArn }}
		role_arn   = "{{ . }}"
{{- end }}
	}
}`))

func NewBundleBuilder() *BundleBuilder {
	return &BundleBuilder{}
}

func (b *BundleBuilder) WithMetadata(metadata map[string]string) *BundleBuilder {
	b.metadata = metadata
	return b
}

func (b *BundleBuilder) WithS3Backend(backendConfig S3BackendConfig) *BundleBuilder {
	b.backendFunc = func() ([]byte, error) {
		configBuf := bytes.Buffer{}
		if err := s3BackendTemplate.Execute(&configBuf, backendConfig); err != nil {
			return nil, fmt.Errorf("error templating s3 backend config: %w", err)
		}
		return configBuf.Bytes(), nil
	}

	return b
}

func (b *BundleBuilder) WithVars(vars map[string]any) *BundleBuilder {
	b.vars = vars
	return b
}

func (b *BundleBuilder) Source(fsys fs.ReadFileFS, root string) *BundleBuilder {
	b.fsys = fsys
	b.root = root
	return b
}

func (b *BundleBuilder) BundleForApply() (zipPath string, retErr error) {
	if b.fsys == nil {
		return "", errors.New("cannot bundle terraform without fs and root")
	}

	zipFile, err := os.CreateTemp("", "tf-bundle.*.zip")
	if err != nil {
		return "", err
	}

	// Cleanup if there's an error building the bundle
	defer func() {
		if retErr != nil {
			_ = os.RemoveAll(zipFile.Name())
		}
	}()

	// Zip the terraform directory
	zipWriter := zip.NewWriter(zipFile)
	if err := fs.WalkDir(b.fsys, b.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := b.fsys.ReadFile(path)
		if err != nil {
			return err
		}

		// Rewrite path to be rooted in zip file
		filePath := strings.TrimPrefix(strings.TrimPrefix(path, b.root), "/")
		w, err := zipWriter.Create(filePath)
		if err != nil {
			return err
		}
		_, err = io.Copy(w, bytes.NewReader(data))
		return err
	}); err != nil {
		return "", err
	}

	// Write backend configuration
	if b.backendFunc != nil {
		backendConfig, err := b.backendFunc()
		if err != nil {
			return "", err
		}

		backendWriter, err := zipWriter.Create("_backend.tf")
		if err != nil {
			return "", err
		}

		if _, err := backendWriter.Write(backendConfig); err != nil {
			return "", err
		}
	}

	// Add the terraform vars to the zip file
	varsWriter, err := zipWriter.Create("terraform.tfvars.json")
	if err != nil {
		return "", err
	}

	varsJSON, err := json.Marshal(b.vars)
	if err != nil {
		return "", err
	}
	if _, err := varsWriter.Write(varsJSON); err != nil {
		return "", err
	}

	// Write additional metadata as separate files to zip
	for k, md := range b.metadata {
		mdWriter, err := zipWriter.Create(path.Join("__metadata", k))
		if err != nil {
			return "", err
		}
		if _, err := mdWriter.Write([]byte(md)); err != nil {
			return "", err
		}
	}

	// Finish writing to zip
	if err := zipWriter.Close(); err != nil {
		return "", err
	}

	return zipFile.Name(), nil
}

func (b *BundleBuilder) BundleForDestroy() (zipPath string, retErr error) {
	if b.fsys == nil {
		return "", errors.New("cannot bundle terraform without fs and root")
	}

	zipFile, err := os.CreateTemp("", "tf-destroy-bundle.*.zip")
	if err != nil {
		return "", err
	}

	// Cleanup if there's an error building the bundle
	defer func() {
		if retErr != nil {
			_ = os.RemoveAll(zipFile.Name())
		}
	}()

	// Only copy versions.tf for destroy because it's needed to determine
	// the versions of terraform providers. Every terraform directory should
	// have a versions.tf at the top level.
	versionsFile, err := b.fsys.Open(path.Join(b.root, "versions.tf"))
	if err != nil {
		return "", err
	}

	zipWriter := zip.NewWriter(zipFile)
	versionsWriter, err := zipWriter.Create("versions.tf")
	if err != nil {
		return "", err
	}

	if _, err := io.Copy(versionsWriter, versionsFile); err != nil {
		return "", err
	}

	// Write backend configuration
	if b.backendFunc != nil {
		backendConfig, err := b.backendFunc()
		if err != nil {
			return "", err
		}

		backendWriter, err := zipWriter.Create("_backend.tf")
		if err != nil {
			return "", err
		}

		if _, err := backendWriter.Write(backendConfig); err != nil {
			return "", err
		}
	}

	// Write additional metadata as separate files to zip
	for k, md := range b.metadata {
		mdWriter, err := zipWriter.Create(path.Join("__metadata", k))
		if err != nil {
			return "", err
		}
		if _, err := mdWriter.Write([]byte(md)); err != nil {
			return "", err
		}
	}

	// Finish writing to zip
	if err := zipWriter.Close(); err != nil {
		return "", err
	}

	return zipFile.Name(), nil
}
