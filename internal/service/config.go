package service

import (
	"context"
	"fmt"

	"github.com/fyltr/angee/api"
	"github.com/fyltr/angee/internal/config"
	"gopkg.in/yaml.v3"
)

// ConfigGet returns the current angee.yaml content.
func (p *Platform) ConfigGet() (*api.ConfigGetResponse, error) {
	cfg, err := p.loadConfig()
	if err != nil {
		return nil, err
	}
	return &api.ConfigGetResponse{Config: cfg}, nil
}

// ConfigSet validates, commits, and optionally deploys a new angee.yaml.
//
// Validates the supplied YAML in memory FIRST, then writes to disk. The
// previous order (write → load → validate) left a broken angee.yaml on disk
// when validation failed, which then prevented the operator from starting
// until someone manually ran `git checkout angee.yaml`.
//
// Holds writeMu so this can't race with a concurrent Deploy / ConfigSet.
func (p *Platform) ConfigSet(ctx context.Context, content, message string, deploy bool) (*api.ConfigSetResponse, error) {
	if content == "" {
		return nil, BadRequest("content is required")
	}
	if message == "" {
		message = "angee-agent: update config"
	}

	// Parse + validate in memory before touching disk.
	var probe config.AngeeConfig
	if err := yaml.Unmarshal([]byte(content), &probe); err != nil {
		return nil, BadRequest(fmt.Sprintf("invalid angee.yaml: %s", err))
	}
	if err := probe.Validate(); err != nil {
		return nil, BadRequest(err.Error())
	}

	p.writeMu.Lock()
	defer p.writeMu.Unlock()

	if err := p.Root.WriteAngeeYAML(content); err != nil {
		return nil, err
	}

	sha, err := p.Root.CommitConfig(message)
	if err != nil {
		return nil, err
	}

	resp := &api.ConfigSetResponse{SHA: sha, Message: message}

	if deploy {
		// Deploy takes writeMu itself — release ours first to avoid a
		// self-deadlock. The on-disk state is already committed at this
		// point, so a brief gap is safe.
		p.writeMu.Unlock()
		result, err := p.Deploy(ctx, "")
		p.writeMu.Lock()
		if err != nil {
			return nil, err
		}
		resp.Deploy = result
	}

	return resp, nil
}
