package service

import (
	"context"
	"fmt"

	"github.com/fyltr/angee/api"
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
func (p *Platform) ConfigSet(ctx context.Context, content, message string, deploy bool) (*api.ConfigSetResponse, error) {
	if content == "" {
		return nil, BadRequest("content is required")
	}
	if message == "" {
		message = "angee-agent: update config"
	}

	// Write raw content
	if err := p.Root.WriteAngeeYAML(content); err != nil {
		return nil, err
	}

	// Verify it parses
	cfg, err := p.Root.LoadAngeeConfig()
	if err != nil {
		return nil, BadRequest(fmt.Sprintf("invalid angee.yaml: %s", err))
	}

	// Structural validation
	if err := cfg.Validate(); err != nil {
		return nil, BadRequest(err.Error())
	}

	// Commit
	sha, err := p.Root.CommitConfig(message)
	if err != nil {
		return nil, err
	}

	resp := &api.ConfigSetResponse{SHA: sha, Message: message}

	if deploy {
		result, err := p.Deploy(ctx, "")
		if err != nil {
			return nil, err
		}
		resp.Deploy = result
	}

	return resp, nil
}
