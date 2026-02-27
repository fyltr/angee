// Package tmpl provides embedded official templates for angee init.
package tmpl

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"text/template"

	"gopkg.in/yaml.v3"
)

//go:embed official
var Official embed.FS

// TemplateParams holds the parameters for rendering an angee.yaml template.
type TemplateParams struct {
	ProjectName    string
	Domain         string
	DBPassword     string
	DBSize         string
	RedisSize      string
	MediaSize      string
	DjangoWorkers  string
	DjangoMemory   string
	DjangoCPU      string
	CeleryWorkers  string
	CeleryMemory   string
	CeleryCPU      string
}

// DefaultParams returns the default template parameters with the given project name.
func DefaultParams(projectName string) TemplateParams {
	return TemplateParams{
		ProjectName:   projectName,
		Domain:        "localhost",
		DBPassword:    "angee",
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
}

// ParamDef describes a single template parameter.
type ParamDef struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Default     string `yaml:"default"`
	Required    bool   `yaml:"required"`
}

// OfficialTemplate represents a named official template with its metadata.
type OfficialTemplate struct {
	Name string
	Meta *TemplateMeta
}

// Render renders the template at the given path (relative to Official FS) with params.
func Render(templatePath string, params TemplateParams) (string, error) {
	tmplContent, err := Official.ReadFile(templatePath)
	if err != nil {
		return "", fmt.Errorf("reading template %s: %w", templatePath, err)
	}

	tmpl, err := template.New("angee").Parse(string(tmplContent))
	if err != nil {
		return "", fmt.Errorf("parsing template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return "", fmt.Errorf("rendering template: %w", err)
	}

	return buf.String(), nil
}

// LoadMeta reads and parses the .angee-template.yaml for a given template path prefix.
func LoadMeta(prefix string) (*TemplateMeta, error) {
	data, err := Official.ReadFile(prefix + "/.angee-template.yaml")
	if err != nil {
		return nil, fmt.Errorf("reading template metadata: %w", err)
	}
	var meta TemplateMeta
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parsing template metadata: %w", err)
	}
	return &meta, nil
}

// RenderOfficial renders an official template by short name (e.g. "angee-django").
func RenderOfficial(name string, params TemplateParams) (string, error) {
	return Render("official/"+name+"/angee.yaml.tmpl", params)
}

// WriteToFile renders a template and writes the result to the destination path.
func WriteToFile(templatePath string, params TemplateParams, destPath string) error {
	content, err := Render(templatePath, params)
	if err != nil {
		return err
	}
	return os.WriteFile(destPath, []byte(content), 0644)
}
