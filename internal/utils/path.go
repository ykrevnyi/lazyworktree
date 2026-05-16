package utils

import (
	"os"
	"path/filepath"
	"strings"
)

// ExpandPath expands ~ and environment variables in a path.
func ExpandPath(path string) (string, error) {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, path[1:])
	}
	return os.ExpandEnv(path), nil
}

// PathContains reports whether child is equal to parent or nested beneath it.
func PathContains(parent, child string) bool {
	if parent == "" || child == "" {
		return false
	}

	parent = filepath.Clean(parent)
	child = filepath.Clean(child)

	if parent == child {
		return true
	}

	return strings.HasPrefix(child, parent+string(filepath.Separator))
}
