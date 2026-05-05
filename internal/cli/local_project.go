package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const (
	localAgoraDirName    = ".agora"
	localProjectFileName = "project.json"
)

type localProjectBinding struct {
	ProjectID   string `json:"projectId"`
	ProjectName string `json:"projectName"`
	Region      string `json:"region"`
	ProjectType string `json:"projectType,omitempty"`
	Template    string `json:"template,omitempty"`
	EnvPath     string `json:"envPath,omitempty"`
}

func resolveLocalProjectFile(root string) string {
	return filepath.Join(root, localAgoraDirName, localProjectFileName)
}

func detectLocalProjectRoot(start string) (string, bool, error) {
	absStart, err := filepath.Abs(start)
	if err != nil {
		return "", false, err
	}
	current := absStart
	for {
		_, err := os.Stat(resolveLocalProjectFile(current))
		if err == nil {
			return current, true, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", false, err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", false, nil
		}
		current = parent
	}
}

func loadLocalProjectBinding(root string) (localProjectBinding, error) {
	raw, err := os.ReadFile(resolveLocalProjectFile(root))
	if err != nil {
		return localProjectBinding{}, err
	}
	var binding localProjectBinding
	if err := json.Unmarshal(raw, &binding); err != nil {
		return localProjectBinding{}, err
	}
	return binding, nil
}

func writeLocalProjectBinding(root string, binding localProjectBinding) error {
	return writeSecureJSON(resolveLocalProjectFile(root), binding)
}

func detectLocalProjectBinding() (localProjectBinding, bool, string, error) {
	return detectLocalProjectBindingFrom("")
}

func detectLocalProjectBindingFrom(start string) (localProjectBinding, bool, string, error) {
	startPath := strings.TrimSpace(start)
	if startPath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return localProjectBinding{}, false, "", err
		}
		startPath = cwd
	}
	root, ok, err := detectLocalProjectRoot(startPath)
	if err != nil || !ok {
		return localProjectBinding{}, false, "", err
	}
	binding, err := loadLocalProjectBinding(root)
	if err != nil {
		return localProjectBinding{}, false, "", err
	}
	return binding, true, root, nil
}
