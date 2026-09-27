package cargo

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
)

type Metadata struct {
	Packages []Package `json:"packages"`
}

type Package struct {
	Name         string       `json:"name"`
	Version      string       `json:"version"`
	ManifestPath string       `json:"manifest_path"`
	Targets      []Target     `json:"targets"`
	Dependencies []Dependency `json:"dependencies"`
}

type Dependency struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Optional bool   `json:"optional"`
}

type Target struct {
	Name      string   `json:"name"`
	Kind      []string `json:"kind"`
	CrateRoot string   `json:"src_path"`
}

func Load(manifest string) (*Metadata, error) {
	return LoadWith("cargo", manifest)
}

func LoadWith(binary, manifest string) (*Metadata, error) {
	if manifest == "" {
		return nil, fmt.Errorf("-manifest is required")
	}
	manifest, err := filepath.Abs(manifest)
	if err != nil {
		return nil, fmt.Errorf("manifest path: %w", err)
	}
	cmd := exec.Command(binary, "metadata", "--format-version", "1", "--no-deps", "--manifest-path", manifest)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("cargo metadata: %w", err)
	}
	var m Metadata
	if err := json.Unmarshal(out, &m); err != nil {
		return nil, fmt.Errorf("decode cargo metadata: %w", err)
	}
	return &m, nil
}

func (m *Metadata) Package(name string) (*Package, error) {
	if name == "" && len(m.Packages) == 1 {
		return &m.Packages[0], nil
	}
	for i := range m.Packages {
		if m.Packages[i].Name == name {
			return &m.Packages[i], nil
		}
	}
	if name == "" {
		return nil, fmt.Errorf("cargo metadata contains %d packages; use -package", len(m.Packages))
	}
	return nil, fmt.Errorf("package %q is not in cargo metadata", name)
}
