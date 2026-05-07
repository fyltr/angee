// Package state provides file-backed operator state for one ANGEE_ROOT.
package state

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	DirName        = "state"
	PortLeasesFile = "port_leases.yaml"
	SecretsFile    = "secrets.yaml"
	LocksDir       = "locks"
	RunsDir        = "runs"
)

// Store is a file-backed state source rooted at $ANGEE_ROOT/state.
type Store struct {
	Root string
}

type PortLease struct {
	Name      string    `yaml:"name" json:"name"`
	Port      int       `yaml:"port" json:"port"`
	Band      string    `yaml:"band,omitempty" json:"band,omitempty"`
	Owner     string    `yaml:"owner,omitempty" json:"owner,omitempty"`
	UpdatedAt time.Time `yaml:"updated_at" json:"updated_at"`
}

type Secret struct {
	Name      string    `yaml:"name" json:"name"`
	Value     string    `yaml:"value" json:"value"`
	Source    string    `yaml:"source,omitempty" json:"source,omitempty"`
	UpdatedAt time.Time `yaml:"updated_at" json:"updated_at"`
}

func New(root string) *Store {
	return &Store{Root: filepath.Join(root, DirName)}
}

func (s *Store) Ensure() error {
	for _, dir := range []string{s.Root, filepath.Join(s.Root, LocksDir), filepath.Join(s.Root, RunsDir)} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("creating %s: %w", dir, err)
		}
	}
	return nil
}

func (s *Store) LoadPortLeases() (map[string]PortLease, error) {
	var leases map[string]PortLease
	if err := s.loadYAML(PortLeasesFile, &leases); err != nil {
		return nil, err
	}
	if leases == nil {
		leases = map[string]PortLease{}
	}
	return leases, nil
}

func (s *Store) SavePortLeases(leases map[string]PortLease) error {
	return s.saveYAML(PortLeasesFile, leases, 0644)
}

func (s *Store) LoadSecrets() (map[string]Secret, error) {
	var secrets map[string]Secret
	if err := s.loadYAML(SecretsFile, &secrets); err != nil {
		return nil, err
	}
	if secrets == nil {
		secrets = map[string]Secret{}
	}
	return secrets, nil
}

func (s *Store) SaveSecrets(secrets map[string]Secret) error {
	return s.saveYAML(SecretsFile, secrets, 0600)
}

func (s *Store) loadYAML(name string, out any) error {
	path := filepath.Join(s.Root, name)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	if len(data) == 0 {
		return nil
	}
	if err := yaml.Unmarshal(data, out); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	return nil
}

func (s *Store) saveYAML(name string, value any, perm os.FileMode) error {
	if err := s.Ensure(); err != nil {
		return err
	}
	data, err := yaml.Marshal(value)
	if err != nil {
		return fmt.Errorf("serializing %s: %w", name, err)
	}
	path := filepath.Join(s.Root, name)
	tmp, err := os.CreateTemp(s.Root, "."+name+"-*")
	if err != nil {
		return fmt.Errorf("creating temp state file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing %s: %w", tmpName, err)
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replacing %s: %w", path, err)
	}
	return os.Chmod(path, perm)
}
