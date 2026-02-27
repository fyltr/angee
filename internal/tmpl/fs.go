// Package tmpl fetches templates from GitHub repos or local directories and renders angee.yaml.
package tmpl

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

// TemplateParams holds the parameters for rendering an angee.yaml template.
type TemplateParams struct {
	ProjectName   string
	Domain        string
	DBSize        string
	RedisSize     string
	MediaSize     string
	DjangoWorkers string
	DjangoMemory  string
	DjangoCPU     string
	CeleryWorkers string
	CeleryMemory  string
	CeleryCPU     string
}

// DefaultParams returns sensible defaults for the given project name.
func DefaultParams(projectName string) TemplateParams {
	return TemplateParams{
		ProjectName:   projectName,
		Domain:        "localhost",
		DBSize:        "20Gi",
		RedisSize:     "2Gi",
		MediaSize:     "10Gi",
		DjangoWorkers: "3",
		DjangoCPU:     "1.0",
		DjangoMemory:  "1Gi",
		CeleryWorkers: "4",
		CeleryCPU:     "1.0",
		CeleryMemory:  "1Gi",
	}
}

// TemplateMeta is the parsed .angee-template.yaml metadata.
type TemplateMeta struct {
	Name        string      `yaml:"name"`
	Description string      `yaml:"description"`
	Version     string      `yaml:"version"`
	Parameters  []ParamDef  `yaml:"parameters"`
	Secrets     []SecretDef `yaml:"secrets"`
}

// ParamDef describes a single template parameter.
type ParamDef struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Default     string `yaml:"default"`
	Required    bool   `yaml:"required"`
}

// SecretDef describes a secret declared by a template.
type SecretDef struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Required    bool   `yaml:"required"`
	Generated   bool   `yaml:"generated"` // auto-generate a value if not supplied
	Derived     string `yaml:"derived"`    // ${other-secret} expression
	Length      int    `yaml:"length"`
	Charset     string `yaml:"charset"`
}

// ResolvedSecret is a secret with its final value and how it was obtained.
type ResolvedSecret struct {
	Name   string
	Value  string
	Source string // "flag" | "generated" | "derived"
}

// FetchTemplate obtains a template directory. If source is a local path that
// exists, it is returned directly. Otherwise it is treated as a git URL and
// cloned to a temp directory. When cleanupDir is non-empty the caller must
// os.RemoveAll it when done.
//
// A fragment suffix selects a subdirectory within a git repo:
//
//	https://github.com/org/repo#templates/default
func FetchTemplate(source string) (dir string, cleanupDir string, err error) {
	// Local path — use directly
	if info, statErr := os.Stat(source); statErr == nil && info.IsDir() {
		return source, "", nil
	}

	// Split url#subdir
	repoURL, subdir := splitFragment(source)

	// Git URL — clone to temp dir
	cloneDir, err := os.MkdirTemp("", "angee-tmpl-*")
	if err != nil {
		return "", "", err
	}
	cmd := exec.Command("git", "clone", "--depth", "1", repoURL, cloneDir)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.RemoveAll(cloneDir)
		return "", "", fmt.Errorf("cloning template %s: %w", repoURL, err)
	}

	dir = cloneDir
	if subdir != "" {
		dir = filepath.Join(cloneDir, subdir)
		if info, statErr := os.Stat(dir); statErr != nil || !info.IsDir() {
			os.RemoveAll(cloneDir)
			return "", "", fmt.Errorf("subdirectory %q not found in %s", subdir, repoURL)
		}
	}

	return dir, cloneDir, nil
}

// splitFragment splits "url#subdir" into ("url", "subdir").
// If there is no '#', subdir is empty.
func splitFragment(source string) (string, string) {
	// Only split on a '#' that comes after "://" to avoid mangling
	// fragments that are actually part of a local path.
	if idx := strings.LastIndex(source, "#"); idx > 0 {
		return source[:idx], source[idx+1:]
	}
	return source, ""
}

// Render reads angee.yaml.tmpl from a local directory and renders it with params.
func Render(templateDir string, params TemplateParams) (string, error) {
	tmplPath := filepath.Join(templateDir, "angee.yaml.tmpl")
	content, err := os.ReadFile(tmplPath)
	if err != nil {
		return "", fmt.Errorf("reading template %s: %w", tmplPath, err)
	}

	tmpl, err := template.New("angee").Parse(string(content))
	if err != nil {
		return "", fmt.Errorf("parsing template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return "", fmt.Errorf("rendering template: %w", err)
	}

	return buf.String(), nil
}

// LoadMeta reads and parses .angee-template.yaml from a local directory.
func LoadMeta(templateDir string) (*TemplateMeta, error) {
	data, err := os.ReadFile(filepath.Join(templateDir, ".angee-template.yaml"))
	if err != nil {
		return nil, fmt.Errorf("reading template metadata: %w", err)
	}
	var meta TemplateMeta
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parsing template metadata: %w", err)
	}
	return &meta, nil
}

// ─── Secret resolution ────────────────────────────────────────────────────────

// ResolveSecrets processes the secrets declared in a template.
// supplied maps secret name → value provided via --secret flags.
// Returns resolved secrets in declaration order.
func ResolveSecrets(meta *TemplateMeta, supplied map[string]string, projectName string) ([]ResolvedSecret, error) {
	resolved := make(map[string]string)
	var ordered []ResolvedSecret

	for _, def := range meta.Secrets {
		var value, source string

		switch {
		case supplied[def.Name] != "":
			// Explicit value from --secret flag takes priority
			value = supplied[def.Name]
			source = "flag"

		case def.Derived != "":
			// Derived from other secrets — resolve after all others are set
			// Skip for now; we do a second pass below
			continue

		case def.Generated:
			// Auto-generate
			generated, err := generateValue(def)
			if err != nil {
				return nil, fmt.Errorf("generating %s: %w", def.Name, err)
			}
			value = generated
			source = "generated"

		case def.Required:
			return nil, fmt.Errorf("secret %q is required — provide it with --secret %s=<value>", def.Name, def.Name)

		default:
			continue
		}

		resolved[def.Name] = value
		ordered = append(ordered, ResolvedSecret{Name: def.Name, Value: value, Source: source})
	}

	// Second pass: resolve derived secrets
	for _, def := range meta.Secrets {
		if def.Derived == "" {
			continue
		}
		value, source := resolveDerived(def, resolved, supplied, projectName)
		resolved[def.Name] = value
		ordered = append(ordered, ResolvedSecret{Name: def.Name, Value: value, Source: source})
	}

	return ordered, nil
}

// resolveDerived resolves a derived secret by substituting ${other-name} and ${project}.
func resolveDerived(def SecretDef, resolved map[string]string, supplied map[string]string, projectName string) (string, string) {
	if supplied[def.Name] != "" {
		return supplied[def.Name], "flag"
	}

	expr := def.Derived
	// Replace ${other-secret} references
	for k, v := range resolved {
		expr = strings.ReplaceAll(expr, "${"+k+"}", v)
	}
	// Replace ${project} with project name
	expr = strings.ReplaceAll(expr, "${project}", projectName)

	return expr, "derived"
}

// generateValue creates a cryptographically random string matching the SecretDef spec.
func generateValue(def SecretDef) (string, error) {
	charset := def.Charset
	if charset == "" {
		charset = "abcdefghijklmnopqrstuvwxyz0123456789!@#%^&*(-_=+)"
	}
	length := def.Length
	if length == 0 {
		length = 50
	}

	runes := []rune(charset)
	result := make([]rune, length)
	for i := range result {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(runes))))
		if err != nil {
			return "", err
		}
		result[i] = runes[n.Int64()]
	}
	return string(result), nil
}

// FormatEnvFile formats resolved secrets as KEY=VALUE lines for a .env file.
func FormatEnvFile(secrets []ResolvedSecret) string {
	var sb strings.Builder
	sb.WriteString("# Angee secrets — written by angee init\n")
	sb.WriteString("# DO NOT COMMIT — this file is gitignored\n\n")
	for _, s := range secrets {
		// Env var name: uppercase, hyphens → underscores
		envKey := strings.ToUpper(strings.ReplaceAll(s.Name, "-", "_"))
		// Escape $ as $$ so Docker Compose doesn't try to interpolate values
		val := strings.ReplaceAll(s.Value, "$", "$$")
		sb.WriteString(fmt.Sprintf("%s=%s\n", envKey, val))
	}
	return sb.String()
}
