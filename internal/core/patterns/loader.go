package patterns

import (
	"eko/internal/core/detector"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/goccy/go-yaml"
)

// Loader handles loading and compiling patterns from files
type Loader struct {
	configDir string
}

// NewLoader creates a new pattern loader
func NewLoader(configDir string) *Loader {
	return &Loader{
		configDir: configDir,
	}
}

// LoadFromFile loads patterns from a YAML file
func (l *Loader) LoadFromFile(filename string) ([]Pattern, error) {
	data, err := os.ReadFile(filepath.Join(l.configDir, filename))
	if err != nil {
		return nil, fmt.Errorf("failed to read pattern file: %w", err)
	}

	var config PatternConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse pattern file: %w", err)
	}

	return config.Patterns, nil
}

// CompilePattern compiles a pattern into a detector-ready format
func (l *Loader) CompilePattern(p Pattern) (*detector.CompiledPattern, error) {
	regex, err := regexp.Compile(p.Regex)
	if err != nil {
		return nil, fmt.Errorf("failed to compile pattern %s: %w", p.Name, err)
	}

	return &detector.CompiledPattern{
		Name:        p.Name,
		Regex:       regex,
		Type:        p.Type,
		Severity:    p.Severity,
		Description: p.Description,
	}, nil
}

// LoadAll loads all patterns from the config directory
func (l *Loader) LoadAll() ([]*detector.CompiledPattern, error) {
	// TODO: Implement directory scanning
	// TODO: Load from both default and custom directories

	return nil, nil
}
