// Package atomicfile replaces files in one step. The temporary file has a
// random name and is created exclusively, so a planted symlink at a
// predictable name such as <file>.tmp is never followed.
package atomicfile

import (
	"os"
	"path/filepath"
)

// Write replaces path with data and the given mode, creating the directory
// if it is missing. A symlink at path is replaced, not followed. The old
// mode of an existing file does not carry over.
func Write(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp.Name(), mode)
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
	}
	return err
}
