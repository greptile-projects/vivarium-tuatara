package checkruns

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
)

// DocumentationConfig declares reproducible verification matrix cells. Each
// target becomes an ordinary check run, so its generated name can be required
// by branch policy without introducing a documentation-specific merge bypass.
type DocumentationConfig struct {
	Version int                  `json:"version"`
	Checks  []DocumentationCheck `json:"checks"`
}

type DocumentationCheck struct {
	Name             string                `json:"name"`
	CollectionID     string                `json:"collection_id"`
	Image            string                `json:"image"`
	Command          string                `json:"command"`
	WorkingDirectory string                `json:"working_directory,omitempty"`
	TimeoutSeconds   int                   `json:"timeout_seconds,omitempty"`
	CPUs             float64               `json:"cpus,omitempty"`
	MemoryMB         int                   `json:"memory_mb,omitempty"`
	StorageMB        int                   `json:"storage_mb,omitempty"`
	Environment      map[string]string     `json:"environment,omitempty"`
	Selectors        []string              `json:"selectors"`
	DependencyPaths  []string              `json:"dependency_paths"`
	Targets          []DocumentationTarget `json:"targets"`
}

type DocumentationTarget struct {
	Version  string `json:"version"`
	Revision string `json:"revision"`
	Source   string `json:"source"`
}

func ParseDocumentationConfig(data []byte, executedRevision string, readPath func(string) ([]byte, error)) (DocumentationConfig, []Definition, error) {
	var config DocumentationConfig
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil || decoder.Decode(&struct{}{}) != io.EOF || config.Version != 1 || len(config.Checks) == 0 || len(config.Checks) > 20 {
		return DocumentationConfig{}, nil, errors.New("invalid documentation check configuration")
	}
	definitions := []Definition{}
	seen := map[string]bool{}
	for _, check := range config.Checks {
		if strings.TrimSpace(check.Name) == "" || len(check.Name) > 70 || len(check.CollectionID) != 32 || len(check.Selectors) == 0 || len(check.Selectors) > 100 || len(check.DependencyPaths) == 0 || len(check.DependencyPaths) > 100 || len(check.Targets) == 0 || len(check.Targets) > 20 {
			return DocumentationConfig{}, nil, errors.New("invalid documentation check")
		}
		paths := append([]string(nil), check.DependencyPaths...)
		sort.Strings(paths)
		hash := sha256.New()
		for _, dependency := range paths {
			clean := filepath.Clean(dependency)
			if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
				return DocumentationConfig{}, nil, errors.New("invalid documentation dependency path")
			}
			body, err := readPath(filepath.ToSlash(clean))
			if err != nil {
				return DocumentationConfig{}, nil, fmt.Errorf("read documentation dependency %s: %w", clean, err)
			}
			hash.Write([]byte(filepath.ToSlash(clean)))
			hash.Write([]byte{0})
			hash.Write(body)
			hash.Write([]byte{0})
		}
		for _, target := range check.Targets {
			name := fmt.Sprintf("docs/%s [%s]", check.Name, target.Version)
			if target.Version == "" || len(target.Version) > 40 || len(target.Revision) != 40 || target.Revision != executedRevision || !map[string]bool{"source": true, "package": true, "release": true}[target.Source] || seen[name] {
				return DocumentationConfig{}, nil, errors.New("invalid documentation target")
			}
			seen[name] = true
			definition := Definition{Name: name, Image: check.Image, Command: check.Command, WorkingDirectory: check.WorkingDirectory, Environment: check.Environment, TimeoutSeconds: check.TimeoutSeconds, CPUs: check.CPUs, MemoryMB: check.MemoryMB, StorageMB: check.StorageMB, Documentation: &DocumentationEvidence{Check: check.Name, CollectionID: check.CollectionID, Version: target.Version, Revision: target.Revision, Source: target.Source, Selectors: check.Selectors, DependencyPaths: paths, DependencySHA256: hex.EncodeToString(hash.Sum(nil))}}
			body, _ := json.Marshal(Config{Version: 1, Checks: []Definition{definition}})
			validated, err := ParseConfig(body)
			if err != nil {
				return DocumentationConfig{}, nil, errors.New("invalid documentation check execution")
			}
			definitions = append(definitions, validated.Checks[0])
		}
	}
	return config, definitions, nil
}
