package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const (
	localAgoraDirName    = ".agora"
	localProjectFileName = "project.json"
)

type localProjectBinding struct {
	ProjectID   string `json:"projectId"`
	ProjectName string `json:"projectName"`
	Region      string `json:"region"`
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
		if _, err := os.Stat(resolveLocalProjectFile(current)); err == nil {
			return current, true, nil
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
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
	cwd, err := os.Getwd()
	if err != nil {
		return localProjectBinding{}, false, "", err
	}
	root, ok, err := detectLocalProjectRoot(cwd)
	if err != nil || !ok {
		return localProjectBinding{}, false, "", err
	}
	binding, err := loadLocalProjectBinding(root)
	if err != nil {
		return localProjectBinding{}, false, "", err
	}
	return binding, true, root, nil
}
