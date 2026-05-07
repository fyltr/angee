// Package copier integrates Copier templates with Angee metadata.
package copier

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Metadata struct {
	Schema      int    `yaml:"schema" json:"schema"`
	Kind        string `yaml:"kind" json:"kind"`
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

type Config struct {
	MinVersion      string         `yaml:"_min_copier_version,omitempty"`
	Subdirectory    string         `yaml:"_subdirectory,omitempty"`
	TemplatesSuffix string         `yaml:"_templates_suffix,omitempty"`
	Angee           Metadata       `yaml:"_angee"`
	Questions       map[string]any `yaml:",inline"`
}

type Template struct {
	Ref    string
	Path   string
	Config Config
}

func Resolve(ref, kind, name string) (*Template, error) {
	path, err := resolvePath(ref, kind, name)
	if err != nil {
		return nil, err
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		return nil, err
	}
	if cfg.Angee.Schema == 0 {
		return nil, fmt.Errorf("template %s is missing _angee metadata", path)
	}
	if kind != "" && cfg.Angee.Kind != kind {
		return nil, fmt.Errorf("template %s has kind %q, want %q", path, cfg.Angee.Kind, kind)
	}
	if name != "" && cfg.Angee.Name != name {
		return nil, fmt.Errorf("template %s has name %q, want %q", path, cfg.Angee.Name, name)
	}
	return &Template{Ref: ref, Path: path, Config: cfg}, nil
}

func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(filepath.Join(path, "copier.yml"))
	if err != nil {
		return Config{}, fmt.Errorf("reading copier.yml: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing copier.yml: %w", err)
	}
	return cfg, nil
}

func Copy(ctx context.Context, tmpl *Template, dst string, data map[string]string, force bool) error {
	if _, err := exec.LookPath("copier"); err != nil {
		return fmt.Errorf("copier executable not found; install Copier 9+ to render templates")
	}
	args := []string{"copy", "--trust", "--defaults", "--quiet", "--answers-file", ".copier-answers.yml"}
	if isGitRepo(tmpl.Path) {
		args = append(args, "--vcs-ref", "HEAD")
	}
	if force {
		args = append(args, "--force")
	}
	for _, key := range sortedKeys(data) {
		args = append(args, "--data", key+"="+data[key])
	}
	args = append(args, tmpl.Path, dst)
	cmd := exec.CommandContext(ctx, "copier", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("copier copy failed: %s", msg)
	}
	return ensureAnswersFile(tmpl, dst, data)
}

func isGitRepo(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil && info.IsDir()
}

func ensureAnswersFile(tmpl *Template, dst string, data map[string]string) error {
	path := filepath.Join(dst, ".copier-answers.yml")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	answers := map[string]any{"_src_path": tmpl.Path}
	if commit := gitHead(tmpl.Path); commit != "" {
		answers["_commit"] = commit
	}
	for key, value := range data {
		answers[key] = value
	}
	body, err := yaml.Marshal(answers)
	if err != nil {
		return fmt.Errorf("serializing copier answers: %w", err)
	}
	return os.WriteFile(path, body, 0644)
}

func gitHead(path string) string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = path
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func Update(ctx context.Context, dst string, data map[string]string, vcsRef ...string) error {
	if _, err := exec.LookPath("copier"); err != nil {
		return fmt.Errorf("copier executable not found; install Copier 9+ to update templates")
	}
	args := []string{"update", "--trust", "--defaults", "--quiet", "--answers-file", ".copier-answers.yml"}
	if len(vcsRef) > 0 && vcsRef[0] != "" {
		args = append(args, "--vcs-ref", vcsRef[0])
	}
	for _, key := range sortedKeys(data) {
		args = append(args, "--data", key+"="+data[key])
	}
	args = append(args, dst)
	if err := runCopier(ctx, args, "update"); err != nil {
		if strings.Contains(err.Error(), "cannot obtain old template references") {
			recopyArgs := []string{"recopy", "--trust", "--defaults", "--force", "--quiet", "--answers-file", ".copier-answers.yml"}
			if len(vcsRef) > 0 && vcsRef[0] != "" {
				recopyArgs = append(recopyArgs, "--vcs-ref", vcsRef[0])
			}
			for _, key := range sortedKeys(data) {
				recopyArgs = append(recopyArgs, "--data", key+"="+data[key])
			}
			recopyArgs = append(recopyArgs, dst)
			return runCopier(ctx, recopyArgs, "recopy")
		}
		return err
	}
	return nil
}

func runCopier(ctx context.Context, args []string, op string) error {
	cmd := exec.CommandContext(ctx, "copier", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("copier %s failed: %s", op, msg)
	}
	return nil
}

func resolvePath(ref, kind, name string) (string, error) {
	candidates := []string{}
	if ref != "" {
		candidates = append(candidates,
			ref,
			filepath.Join("examples", "templates", ref),
			filepath.Join("templates", ref),
		)
	}
	if kind != "" && name != "" {
		plural := kind + "s"
		candidates = append(candidates,
			filepath.Join("examples", "templates", plural, name),
			filepath.Join("templates", plural, name),
			filepath.Join(plural, name),
		)
	}
	for _, candidate := range candidates {
		path := candidate
		if !filepath.IsAbs(path) {
			abs, err := filepath.Abs(path)
			if err == nil {
				path = abs
			}
		}
		info, err := os.Stat(path)
		if err == nil && info.IsDir() {
			if _, err := os.Stat(filepath.Join(path, "copier.yml")); err == nil {
				return path, nil
			}
		}
	}
	if ref == "" {
		ref = kind + "s/" + name
	}
	return "", fmt.Errorf("template %q not found", ref)
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
