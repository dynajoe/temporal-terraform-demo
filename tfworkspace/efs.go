package tfworkspace

import (
	"embed"
	"os"
	"path"
)

func extractEmbeddedTerraform(efs embed.FS, src string, dst string) error {
	entries, err := efs.ReadDir(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, os.ModePerm); err != nil {
		return err
	}

	for _, e := range entries {
		if e.IsDir() {
			if err := extractEmbeddedTerraform(efs, path.Join(src, e.Name()), path.Join(dst, e.Name())); err != nil {
				return err
			}
			continue
		}

		data, err := efs.ReadFile(path.Join(src, e.Name()))
		if err != nil {
			return err
		}

		if err := os.WriteFile(path.Join(dst, e.Name()), data, os.ModePerm); err != nil {
			return err
		}
	}

	return nil
}
