package verdandi

import (
	"os"
	"path/filepath"
	"sync"
)

var fileLocks sync.Map

func lockForPath(path string) *sync.Mutex {
	clean := filepath.Clean(path)
	value, _ := fileLocks.LoadOrStore(clean, &sync.Mutex{})
	return value.(*sync.Mutex)
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	file, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)

	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Chmod(perm); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}
