package proccompose

import "gopkg.in/yaml.v3"

type File struct {
	Version   string             `yaml:"version"`
	Processes map[string]Process `yaml:"processes,omitempty"`
}

type Process struct {
	Command     string            `yaml:"command,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty"`
	WorkingDir  string            `yaml:"working_dir,omitempty"`
	DependsOn   []string          `yaml:"depends_on,omitempty"`
}

func Marshal(file File) ([]byte, error) {
	return yaml.Marshal(file)
}
