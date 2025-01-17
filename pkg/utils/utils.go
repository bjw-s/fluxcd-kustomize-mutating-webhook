package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var AppConfig struct {
	Mu     sync.RWMutex
	Config map[string]string
}

func ReadConfigMap(directory string) error {
	config := make(map[string]string)
	files, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("error reading directory: %w", err)
	}

	for _, file := range files {
		if file.IsDir() || strings.HasPrefix(file.Name(), ".") {
			continue
		}

		fullPath := filepath.Join(directory, file.Name())
		value, err := os.ReadFile(fullPath)
		if err != nil {
			return fmt.Errorf("error reading file %s: %w", fullPath, err)
		}
		config[file.Name()] = string(value)
	}

	if len(config) == 0 {
		return fmt.Errorf("no configuration found")
	}

	AppConfig.Mu.Lock()
	AppConfig.Config = config
	AppConfig.Mu.Unlock()

	return nil
}

func GetAppConfig(key string) (string, bool) {
	AppConfig.Mu.RLock()
	defer AppConfig.Mu.RUnlock()
	value, ok := AppConfig.Config[key]
	return value, ok
}

func EscapeJsonPointer(value string) string {
	value = strings.ReplaceAll(value, "~", "~0")
	value = strings.ReplaceAll(value, "/", "~1")
	return value
}
