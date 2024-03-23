package duplicator

import (
	"os"
	"path/filepath"
)

func CloneDirectory(srcDir, dstDir string) error {
	// Read the contents of the source directory
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}

	err = os.MkdirAll(dstDir, os.ModePerm)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(srcDir, entry.Name())
		dstPath := filepath.Join(dstDir, entry.Name())

		if entry.IsDir() {
			// If the entry is a directory, recursively clone it
			err = CloneDirectory(srcPath, dstPath)
			if err != nil {
				return err
			}
		} else {
			// If the entry is a file, clone it
			err = CloneFile(srcPath, dstPath)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
