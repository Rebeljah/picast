package fileutil

import (
	"os"
	"path/filepath"
)

// ReplaceFileContents atomically replaces file contents using temp file and rename.
// This version is optimized for Linux and assumes same-filesystem operation.
func ReplaceFileContents(filename string, buf []byte) error {
	// Create temp file in same directory (ensures same filesystem)
	dir := filepath.Dir(filename)
	tmpfile, err := os.CreateTemp(dir, ".tmp_"+filepath.Base(filename)+"_")
	if err != nil {
		return err
	}
	tmpName := tmpfile.Name()

	// Clean up temp file if we fail (except on successful rename)
	defer func() {
		if _, err := os.Stat(tmpName); !os.IsNotExist(err) {
			os.Remove(tmpName)
		}
	}()

	// Write new content
	if _, err := tmpfile.Write(buf); err != nil {
		return err
	}

	// Set permissions (0600 for security)
	if err := tmpfile.Chmod(0600); err != nil {
		return err
	}

	// Ensure data is flushed to disk
	if err := tmpfile.Sync(); err != nil {
		return err
	}
	if err := tmpfile.Close(); err != nil {
		return err
	}

	// Atomic rename - will always work on Linux when same filesystem
	return os.Rename(tmpName, filename)
}

// TouchFile creates an empty file at the given path, creating parent directories if needed.
// Similar to Unix 'touch' command but with directory creation.
// Returns:
//   - (true, nil) if file was created
//   - (false, nil) if file already existed
//   - (false, error) if any error occurred
func TouchFile(filename string) (bool, error) {
	// Create parent directories if they don't exist
	dir := filepath.Dir(filename)
	if dir != "." { // Don't try to create current directory
		if err := os.MkdirAll(dir, 0755); err != nil {
			return false, err
		}
	}

	// Check if file already exists
	if _, err := os.Stat(filename); err == nil {
		return false, nil // File exists, nothing to do
	} else if !os.IsNotExist(err) {
		return false, err // Some other stat error
	}

	// Create the file
	file, err := os.Create(filename)
	if err != nil {
		return false, err
	}
	defer file.Close()

	// Set secure permissions (0600)
	if err := file.Chmod(0600); err != nil {
		return false, err
	}

	return true, nil
}
