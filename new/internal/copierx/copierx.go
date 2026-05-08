package copierx

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/fyltr/angee/internal/manifest"
	"gopkg.in/yaml.v3"
)

type Inputs map[string]string

type CopyRequest struct {
	Template string
	Dest     string
	Inputs   Inputs
}

type UpdateRequest struct {
	Template string
	Dest     string
	Inputs   Inputs
}

type Renderer interface {
	Copy(ctx context.Context, req CopyRequest) error
	Update(ctx context.Context, req UpdateRequest) error
}

type Metadata struct {
	Kind           string                          `yaml:"kind"`
	Name           string                          `yaml:"name"`
	InstanceNaming InstanceNaming                  `yaml:"instance_naming"`
	Inputs         map[string]Input                `yaml:"inputs"`
	Sources        map[string]TemplateSource       `yaml:"sources"`
	ChainRoot      string                          `yaml:"chain_root"`
	Chain          []ChainEntry                    `yaml:"chain"`
	Persist        map[string]manifest.PersistPath `yaml:"persist"`
}

type InstanceNaming struct {
	Pattern   string `yaml:"pattern"`
	Fallback  string `yaml:"fallback"`
	MaxLength int    `yaml:"max_length"`
}

type Input struct {
	Type      string `yaml:"type"`
	Required  bool   `yaml:"required"`
	Default   any    `yaml:"default"`
	Immutable bool   `yaml:"immutable"`
}

type TemplateSource struct {
	Source   string `yaml:"source"`
	Mode     string `yaml:"mode"`
	Ref      string `yaml:"ref"`
	Branch   string `yaml:"branch"`
	Subpath  string `yaml:"subpath"`
	Optional bool   `yaml:"optional"`
}

type ChainEntry struct {
	Template string            `yaml:"template"`
	Root     string            `yaml:"root"`
	Workdir  string            `yaml:"workdir"`
	Inputs   map[string]string `yaml:"inputs"`
}

type config struct {
	Subdirectory string           `yaml:"_subdirectory"`
	Suffix       string           `yaml:"_templates_suffix"`
	AnswersFile  string           `yaml:"_answers_file"`
	Angee        Metadata         `yaml:"_angee"`
	Defaults     Inputs           `yaml:"-"`
	Questions    map[string]Input `yaml:"-"`
}

type LocalRenderer struct{}

func ReadMetadata(templatePath string) (Metadata, error) {
	cfg, err := readConfig(templatePath)
	if err != nil {
		return Metadata{}, err
	}
	return cfg.Angee, nil
}

func (LocalRenderer) Copy(ctx context.Context, req CopyRequest) error {
	cfg, err := readConfig(req.Template)
	if err != nil {
		return err
	}
	return render(ctx, req.Template, req.Dest, cfg, req.Inputs)
}

func (r LocalRenderer) Update(ctx context.Context, req UpdateRequest) error {
	return r.Copy(ctx, CopyRequest{Template: req.Template, Dest: req.Dest, Inputs: req.Inputs})
}

func TemplateInputs(templatePath string, inputs Inputs) (Inputs, error) {
	cfg, err := readConfig(templatePath)
	if err != nil {
		return nil, err
	}
	return mergeInputs(cfg, inputs), nil
}

func TemplateQuestions(templatePath string) (map[string]Input, Inputs, error) {
	cfg, err := readConfig(templatePath)
	if err != nil {
		return nil, nil, err
	}
	return cfg.Questions, cfg.Defaults, nil
}

func readConfig(templatePath string) (config, error) {
	data, err := os.ReadFile(filepath.Join(templatePath, "copier.yml"))
	if err != nil {
		return config{}, err
	}
	var cfg config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return config{}, err
	}
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err == nil {
		cfg.Defaults = defaultsFromRaw(raw)
		cfg.Questions = questionsFromRaw(raw)
	}
	if cfg.Subdirectory == "" {
		cfg.Subdirectory = "."
	}
	if cfg.Suffix == "" {
		cfg.Suffix = ".jinja"
	}
	if cfg.AnswersFile == "" {
		cfg.AnswersFile = ".copier-answers.yml"
	}
	return cfg, nil
}

func questionsFromRaw(raw map[string]any) map[string]Input {
	questions := map[string]Input{}
	for key, value := range raw {
		if strings.HasPrefix(key, "_") {
			continue
		}
		body, ok := value.(map[string]any)
		if !ok {
			continue
		}
		encoded, err := yaml.Marshal(body)
		if err != nil {
			continue
		}
		var input Input
		if err := yaml.Unmarshal(encoded, &input); err != nil {
			continue
		}
		questions[key] = input
	}
	return questions
}

func render(ctx context.Context, templatePath, dest string, cfg config, inputs Inputs) error {
	mergedInputs := mergeInputs(cfg, inputs)
	src := filepath.Join(templatePath, cfg.Subdirectory)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	if err := filepath.WalkDir(src, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		renderedRel := renderString(strings.TrimSuffix(rel, cfg.Suffix), mergedInputs)
		target := filepath.Join(dest, renderedRel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.HasSuffix(path, cfg.Suffix) {
			data = []byte(renderString(string(data), mergedInputs))
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	}); err != nil {
		return err
	}
	answers, err := yaml.Marshal(map[string]string(mergedInputs))
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dest, cfg.AnswersFile), answers, 0o644)
}

func mergeInputs(cfg config, inputs Inputs) Inputs {
	mergedInputs := Inputs{}
	for key, value := range cfg.Defaults {
		mergedInputs[key] = value
	}
	for key, value := range inputs {
		mergedInputs[key] = value
	}
	return mergedInputs
}

func defaultsFromRaw(raw map[string]any) Inputs {
	defaults := Inputs{}
	for key, value := range raw {
		if strings.HasPrefix(key, "_") {
			continue
		}
		body, ok := value.(map[string]any)
		if !ok {
			continue
		}
		defaultValue, ok := body["default"]
		if !ok {
			continue
		}
		defaults[key] = fmt.Sprint(defaultValue)
	}
	return defaults
}

var expressionRE = regexp.MustCompile(`\{\{\s*([^{}]+?)\s*\}\}`)

func renderString(input string, values Inputs) string {
	return expressionRE.ReplaceAllStringFunc(input, func(match string) string {
		expr := strings.TrimSpace(match[2 : len(match)-2])
		name, filter, hasFilter := strings.Cut(expr, "|")
		value := values[strings.TrimSpace(name)]
		if hasFilter {
			value = applyFilter(value, strings.TrimSpace(filter))
		}
		return value
	})
}

func applyFilter(value, filter string) string {
	if strings.HasPrefix(filter, "replace(") && strings.HasSuffix(filter, ")") {
		args := strings.TrimSuffix(strings.TrimPrefix(filter, "replace("), ")")
		from, to, ok := strings.Cut(args, ",")
		if !ok {
			return value
		}
		from = strings.Trim(strings.TrimSpace(from), "'\"")
		to = strings.Trim(strings.TrimSpace(to), "'\"")
		return strings.ReplaceAll(value, from, to)
	}
	return value
}

func ValidateMetadata(path string, wantKind string) (Metadata, error) {
	metadata, err := ReadMetadata(path)
	if err != nil {
		return Metadata{}, err
	}
	if metadata.Kind != wantKind {
		return Metadata{}, fmt.Errorf("template kind %q does not match %q", metadata.Kind, wantKind)
	}
	return metadata, nil
}
