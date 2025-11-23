package patterns

import (
	"eko/internal/helpers/logger"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/goccy/go-yaml"
)

// Loader handles loading and compiling patterns from files
type Loader struct {
	configDir         string
	customPatternsDir string
	cache             map[string]*CompiledPattern
	cacheMu           sync.RWMutex
}

// NewLoader creates a new pattern loader
func NewLoader(configDir string, customPatternsDir string) *Loader {
	return &Loader{
		configDir:         configDir,
		customPatternsDir: customPatternsDir,
		cache:             make(map[string]*CompiledPattern),
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

// CompilePattern compiles a pattern into a detector-ready format with caching
func (l *Loader) CompilePattern(p Pattern) (*CompiledPattern, error) {
	// Check cache first
	l.cacheMu.RLock()
	if cached, exists := l.cache[p.Name]; exists {
		l.cacheMu.RUnlock()
		return cached, nil
	}
	l.cacheMu.RUnlock()

	// Validate pattern
	if err := l.validatePattern(p); err != nil {
		return nil, fmt.Errorf("invalid pattern %s: %w", p.Name, err)
	}

	// Compile regex
	regex, err := regexp.Compile(p.Regex)
	if err != nil {
		return nil, fmt.Errorf("failed to compile pattern %s: %w", p.Name, err)
	}

	compiled := &CompiledPattern{
		Name:        p.Name,
		Regex:       regex,
		Type:        p.Type,
		Severity:    p.Severity,
		Description: p.Description,
	}

	// Cache the compiled pattern
	l.cacheMu.Lock()
	l.cache[p.Name] = compiled
	l.cacheMu.Unlock()

	return compiled, nil
}

// validatePattern validates a pattern definition
func (l *Loader) validatePattern(p Pattern) error {
	if p.Name == "" {
		return fmt.Errorf("pattern name cannot be empty")
	}
	if p.Regex == "" {
		return fmt.Errorf("pattern regex cannot be empty")
	}
	if p.Type == "" {
		return fmt.Errorf("pattern type cannot be empty")
	}
	if p.Severity == "" {
		return fmt.Errorf("pattern severity cannot be empty")
	}

	// Validate type
	validTypes := map[string]bool{
		TypePII:                  true,
		TypeCredential:           true,
		TypeFinancial:            true,
		TypeBusinessIntelligence: true,
	}
	if !validTypes[p.Type] {
		return fmt.Errorf("invalid pattern type: %s", p.Type)
	}

	// Validate severity
	validSeverities := map[string]bool{
		SeverityBlock: true,
		SeverityWarn:  true,
		SeverityLog:   true,
	}
	if !validSeverities[p.Severity] {
		return fmt.Errorf("invalid severity level: %s", p.Severity)
	}

	return nil
}

// LoadAll loads all patterns from the config directory and custom patterns directory
func (l *Loader) LoadAll() ([]*CompiledPattern, error) {
	var allPatterns []Pattern
	loadedFiles := 0

	// Load from default config file/directory
	if l.configDir != "" {
		patterns, filesLoaded, err := l.loadFromDirectory(l.configDir, "default patterns")
		if err != nil {
			logger.Warn("Failed to load default patterns", logger.Fields{
				"error": err.Error(),
				"path":  l.configDir,
			})
		} else {
			allPatterns = append(allPatterns, patterns...)
			loadedFiles += filesLoaded
		}
	}

	// Load from custom patterns directory
	if l.customPatternsDir != "" {
		patterns, filesLoaded, err := l.loadFromDirectory(l.customPatternsDir, "custom patterns")
		if err != nil {
			logger.Warn("Failed to load custom patterns", logger.Fields{
				"error": err.Error(),
				"path":  l.customPatternsDir,
			})
		} else {
			allPatterns = append(allPatterns, patterns...)
			loadedFiles += filesLoaded
		}
	}

	if len(allPatterns) == 0 {
		return nil, fmt.Errorf("no patterns loaded from any directory")
	}

	// Compile all patterns
	compiled := make([]*CompiledPattern, 0, len(allPatterns))
	failedCount := 0

	for _, p := range allPatterns {
		cp, err := l.CompilePattern(p)
		if err != nil {
			logger.Warn("Failed to compile pattern", logger.Fields{
				"pattern": p.Name,
				"error":   err.Error(),
			})
			failedCount++
			continue
		}
		compiled = append(compiled, cp)
	}

	logger.Info("Pattern loading completed", logger.Fields{
		"total_files":    loadedFiles,
		"total_patterns": len(allPatterns),
		"compiled":       len(compiled),
		"failed":         failedCount,
		"cached":         len(l.cache),
	})

	return compiled, nil
}

// loadFromDirectory loads all YAML files from a directory
func (l *Loader) loadFromDirectory(dir string, source string) ([]Pattern, int, error) {
	// Check if directory exists
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, fmt.Errorf("directory does not exist: %s", dir)
		}
		return nil, 0, fmt.Errorf("failed to access directory: %w", err)
	}

	var allPatterns []Pattern
	filesLoaded := 0

	// If it's a file, load it directly
	if !info.IsDir() {
		patterns, err := l.loadYAMLFile(dir)
		if err != nil {
			return nil, 0, err
		}
		logger.Info("Loaded patterns from file", logger.Fields{
			"source": source,
			"file":   dir,
			"count":  len(patterns),
		})
		return patterns, 1, nil
	}

	// Walk the directory and load all YAML files
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Only process YAML files
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}

		patterns, err := l.loadYAMLFile(path)
		if err != nil {
			logger.Warn("Failed to load pattern file", logger.Fields{
				"file":  path,
				"error": err.Error(),
			})
			return nil // Continue loading other files
		}

		allPatterns = append(allPatterns, patterns...)
		filesLoaded++

		logger.Debug("Loaded pattern file", logger.Fields{
			"file":   filepath.Base(path),
			"count":  len(patterns),
			"source": source,
		})

		return nil
	})

	if err != nil {
		return nil, 0, fmt.Errorf("failed to walk directory %s: %w", dir, err)
	}

	return allPatterns, filesLoaded, nil
}

// loadYAMLFile loads patterns from a single YAML file
func (l *Loader) loadYAMLFile(path string) ([]Pattern, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var config PatternConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return config.Patterns, nil
}

// ClearCache clears the pattern cache
func (l *Loader) ClearCache() {
	l.cacheMu.Lock()
	defer l.cacheMu.Unlock()
	l.cache = make(map[string]*CompiledPattern)
	logger.Debug("Pattern cache cleared", logger.Fields{})
}

// GetCacheSize returns the number of cached patterns
func (l *Loader) GetCacheSize() int {
	l.cacheMu.RLock()
	defer l.cacheMu.RUnlock()
	return len(l.cache)
}
