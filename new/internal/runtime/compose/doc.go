package compose

import "gopkg.in/yaml.v3"

type File struct {
	Name     string             `yaml:"name,omitempty"`
	Services map[string]Service `yaml:"services,omitempty"`
	Volumes  map[string]Volume  `yaml:"volumes,omitempty"`
}

type Service struct {
	Image       string            `yaml:"image,omitempty"`
	Build       any               `yaml:"build,omitempty"`
	Command     []string          `yaml:"command,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty"`
	Ports       []string          `yaml:"ports,omitempty"`
	Volumes     []string          `yaml:"volumes,omitempty"`
	WorkingDir  string            `yaml:"working_dir,omitempty"`
}

type Volume struct {
	Driver string `yaml:"driver,omitempty"`
	Name   string `yaml:"name,omitempty"`
}

func Marshal(file File) ([]byte, error) {
	return yaml.Marshal(file)
}
