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
	Dev           bool // use angee.dev.yaml.tmpl (infrastructure only)
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

	// Runtime selects the language framework adapter for project-mode
	// init (django-angee R-15/R-16). Empty for compose-mode templates.
	// When set AND Services is empty, `angee init --dev` skips
	// docker-compose generation and runs the runtime-only branch
	// (see cli/init.go).
	Runtime string `yaml:"runtime,omitempty"`

	// Services is informational in compose-mode and a sentinel in
	// project-mode: an empty list together with Runtime != "" triggers
	// runtime-only init.
	Services []string `yaml:"services,omitempty"`

	// Fixtures are loaded post-migrate by the runtime adapter
	// (e.g. `manage.py loaddata` for Django). Paths are relative to the
	// template root. Order matters — they're loaded in the listed order.
	Fixtures []string `yaml:"fixtures,omitempty"`
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
	Derived     string `yaml:"derived"`   // ${other-secret} expression
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
//
// Only https://, http:// (for localhost dev), git@host:org/repo (SSH), and
// local filesystem paths are accepted; anything else is rejected. The clone
// itself runs with `protocol.allow=...` to defeat URL-smuggling attacks.
func FetchTemplate(source string) (dir string, cleanupDir string, err error) {
	// Local path — use directly
	if info, statErr := os.Stat(source); statErr == nil && info.IsDir() {
		return source, "", nil
	}

	// Split url#subdir
	repoURL, subdir := splitFragment(source)

	if err := validateTemplateURL(repoURL); err != nil {
		return "", "", err
	}

	// Git URL — clone to temp dir.
	cloneDir, err := os.MkdirTemp("", "angee-tmpl-*")
	if err != nil {
		return "", "", err
	}
	// `--` ensures repoURL is treated as a positional arg even if it begins
	// with `-` (defence in depth alongside the validateTemplateURL prefix
	// check).
	cmd := exec.Command("git",
		"-c", "protocol.allow=https:always,http:user,ssh:user,file:user,git:never",
		"clone", "--depth", "1", "--", repoURL, cloneDir,
	)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		_ = os.RemoveAll(cloneDir)
		return "", "", fmt.Errorf("cloning template %s: %w", repoURL, err)
	}

	dir = cloneDir
	if subdir != "" {
		dir = filepath.Join(cloneDir, subdir)
		if info, statErr := os.Stat(dir); statErr != nil || !info.IsDir() {
			_ = os.RemoveAll(cloneDir)
			return "", "", fmt.Errorf("subdirectory %q not found in %s", subdir, repoURL)
		}
	}

	return dir, cloneDir, nil
}

// validateTemplateURL refuses anything that isn't an obvious git URL or
// SSH-form. We reject schemes like `ext::` or `--upload-pack=...` that have
// historically been the source of git-clone CVEs, and disallow leading `-`.
func validateTemplateURL(u string) error {
	if u == "" {
		return fmt.Errorf("empty template URL")
	}
	if strings.HasPrefix(u, "-") {
		return fmt.Errorf("template URL must not start with '-'")
	}
	switch {
	case strings.HasPrefix(u, "https://"):
		return nil
	case strings.HasPrefix(u, "http://"):
		return nil
	case strings.HasPrefix(u, "ssh://"):
		return nil
	// SSH "scp-like" form: user@host:path (e.g. git@github.com:org/repo).
	case strings.Contains(u, "@") && strings.Contains(u, ":") && !strings.Contains(u, "://"):
		return nil
	}
	return fmt.Errorf("unsupported template URL scheme: %s (allowed: https://, http://, ssh://, user@host:path)", u)
}

// splitFragment splits "url#subdir" into ("url", "subdir").
// Only splits on a '#' that comes after "://" so local paths containing '#'
// in their name are not mishandled.
func splitFragment(source string) (string, string) {
	scheme := strings.Index(source, "://")
	if scheme < 0 {
		return source, ""
	}
	rest := source[scheme+3:]
	idx := strings.Index(rest, "#")
	if idx < 0 {
		return source, ""
	}
	abs := scheme + 3 + idx
	return source[:abs], source[abs+1:]
}

// Render reads the appropriate angee.yaml template from a local directory and
// renders it with params. When params.Dev is true, angee.dev.yaml.tmpl is used
// instead of angee.yaml.tmpl.
func Render(templateDir string, params TemplateParams) (string, error) {
	tmplName := "angee.yaml.tmpl"
	if params.Dev {
		tmplName = "angee.dev.yaml.tmpl"
	}
	tmplPath := filepath.Join(templateDir, tmplName)
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

// PromptFunc asks the user for a secret value interactively.
// It receives the secret definition and returns the user-supplied value.
// Return empty string to skip (will error if required).
type PromptFunc func(def SecretDef) (string, error)

// ResolveSecrets processes the secrets declared in a template.
// supplied maps secret name → value provided via --secret flags.
// promptFn is called for required non-generated secrets that aren't supplied.
// Pass nil for non-interactive mode (errors on missing required secrets).
// Returns resolved secrets in declaration order.
func ResolveSecrets(meta *TemplateMeta, supplied map[string]string, projectName string, promptFn PromptFunc) ([]ResolvedSecret, error) {
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
			// Required secret not supplied and not generated — prompt interactively
			if promptFn != nil {
				prompted, err := promptFn(def)
				if err != nil {
					return nil, err
				}
				if prompted != "" {
					value = prompted
					source = "prompt"
					break
				}
			}
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

// CopyTemplateFiles copies *.tmpl files (excluding angee.yaml.tmpl) from the
// template source root into ANGEE_ROOT so they are available for agent file
// rendering at deploy time. Existing files are not overwritten.
func CopyTemplateFiles(templateDir, angeeRoot string) error {
	entries, err := os.ReadDir(templateDir)
	if err != nil {
		return fmt.Errorf("reading template dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".tmpl") || name == "angee.yaml.tmpl" || name == "angee.dev.yaml.tmpl" {
			continue
		}

		dstPath := filepath.Join(angeeRoot, name)
		// Don't overwrite existing files
		if _, err := os.Stat(dstPath); err == nil {
			continue
		}

		data, err := os.ReadFile(filepath.Join(templateDir, name))
		if err != nil {
			return fmt.Errorf("reading %s: %w", name, err)
		}
		if err := os.WriteFile(dstPath, data, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", dstPath, err)
		}
	}

	return nil
}

// CopyAgentFiles copies agent workspace files (AGENTS.md, etc.) from the
// template's agents/ directory into the ANGEE_ROOT agents/ directory.
// Only copies files that exist in the template; does not overwrite existing files.
func CopyAgentFiles(templateDir, angeeRoot string) error {
	agentsDir := filepath.Join(templateDir, "agents")
	if _, err := os.Stat(agentsDir); os.IsNotExist(err) {
		return nil // no agents directory in template — nothing to copy
	}

	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		return fmt.Errorf("reading template agents dir: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		agentName := entry.Name()
		srcDir := filepath.Join(agentsDir, agentName)
		dstDir := filepath.Join(angeeRoot, "agents", agentName, "workspace")

		if err := os.MkdirAll(dstDir, 0755); err != nil {
			return fmt.Errorf("creating agent workspace %s: %w", agentName, err)
		}

		// Copy all files from the template agent dir
		files, err := os.ReadDir(srcDir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() {
				continue // skip subdirectories for now
			}
			srcPath := filepath.Join(srcDir, f.Name())
			dstPath := filepath.Join(dstDir, f.Name())

			// Don't overwrite existing files
			if _, err := os.Stat(dstPath); err == nil {
				continue
			}

			data, err := os.ReadFile(srcPath)
			if err != nil {
				return fmt.Errorf("reading %s: %w", srcPath, err)
			}
			if err := os.WriteFile(dstPath, data, 0644); err != nil {
				return fmt.Errorf("writing %s: %w", dstPath, err)
			}
		}
	}

	return nil
}

// FormatDevEnvFile generates a Django-compatible .env for local development.
// It maps angee secret names to Django env var names, adds localhost connection
// strings, and includes the angee-format DB_PASSWORD so that docker-compose
// variable substitution works when docker-compose.yaml lives alongside this file.
func FormatDevEnvFile(secrets []ResolvedSecret) string {
	m := make(map[string]string)
	for _, s := range secrets {
		m[s.Name] = s.Value
	}

	dbPass := m["db-password"]

	var sb strings.Builder
	sb.WriteString("# Generated by angee init --dev\n")
	sb.WriteString("# DO NOT COMMIT — this file is gitignored\n\n")

	sb.WriteString("# ── Django ────────────────────────────────────────────────────────────────────\n")
	devEnvLine(&sb, "DJANGO_SECRET_KEY", m["django-secret-key"])
	devEnvLine(&sb, "DJANGO_SQIDS_SALT", m["django-sqids-salt"])
	devEnvLine(&sb, "PYTHONPATH", "./src")
	sb.WriteString("\n")

	sb.WriteString("# ── Database ──────────────────────────────────────────────────────────────────\n")
	devEnvLine(&sb, "POSTGRES_HOST", "127.0.0.1")
	devEnvLine(&sb, "POSTGRES_PORT", "5432")
	devEnvLine(&sb, "POSTGRES_DB", "fyltr")
	devEnvLine(&sb, "POSTGRES_USER", "postgres")
	devEnvLine(&sb, "POSTGRES_PASSWORD", dbPass)
	devEnvLine(&sb, "DATABASE_URL", fmt.Sprintf("postgresql://postgres:%s@127.0.0.1:5432/fyltr", dbPass))
	sb.WriteString("\n")

	sb.WriteString("# ── Redis / Celery ────────────────────────────────────────────────────────────\n")
	devEnvLine(&sb, "REDIS_URL", "redis://127.0.0.1:6379/0")
	devEnvLine(&sb, "CELERY_BROKER_URL", "redis://127.0.0.1:6379/0")
	sb.WriteString("\n")

	sb.WriteString("# ── AI providers ──────────────────────────────────────────────────────────────\n")
	devEnvLine(&sb, "ANTHROPIC_API_KEY", m["anthropic-api-key"])
	sb.WriteString("\n")

	// DB_PASSWORD is referenced by docker-compose.yaml as ${DB_PASSWORD}
	// (compiled from ${secret:db-password}).
	sb.WriteString("# ── Docker Compose interpolation (do not remove) ─────────────────────────────\n")
	devEnvLine(&sb, "DB_PASSWORD", dbPass)
	sb.WriteString("\n")

	return sb.String()
}

func devEnvLine(sb *strings.Builder, key, value string) {
	sb.WriteString(key)
	sb.WriteByte('=')
	sb.WriteString(value)
	sb.WriteByte('\n')
}

// FormatEnvFile formats resolved secrets as KEY=VALUE lines for a .env file.
//
// Values are double-quoted and special characters (`"`, `\`, newlines) are
// escaped — the previous unquoted format silently broke when a generator or
// user-supplied secret contained whitespace, quotes, or hash characters that
// some .env parsers interpret as comments.
func FormatEnvFile(secrets []ResolvedSecret) string {
	var sb strings.Builder
	sb.WriteString("# Angee secrets — written by angee init\n")
	sb.WriteString("# DO NOT COMMIT — this file is gitignored\n\n")
	for _, s := range secrets {
		envKey := strings.ToUpper(strings.ReplaceAll(s.Name, "-", "_"))
		// Escape $ → $$ so docker-compose interpolation leaves the literal
		// value intact, then double-quote with backslash escaping for safety.
		val := strings.ReplaceAll(s.Value, "$", "$$")
		fmt.Fprintf(&sb, "%s=%s\n", envKey, escapeEnvValue(val))
	}
	return sb.String()
}

// escapeEnvValue produces a `.env`-safe representation of v.
// Empty strings render as `""` rather than nothing so the parser can tell the
// difference between "set to empty" and "missing".
func escapeEnvValue(v string) string {
	var sb strings.Builder
	sb.Grow(len(v) + 2)
	sb.WriteByte('"')
	for _, r := range v {
		switch r {
		case '\\', '"':
			sb.WriteByte('\\')
			sb.WriteRune(r)
		case '\n':
			sb.WriteString(`\n`)
		case '\r':
			sb.WriteString(`\r`)
		case '\t':
			sb.WriteString(`\t`)
		default:
			sb.WriteRune(r)
		}
	}
	sb.WriteByte('"')
	return sb.String()
}
