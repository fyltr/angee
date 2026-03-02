package service

import (
	"context"
	"fmt"

	"github.com/fyltr/angee/api"
	"github.com/fyltr/angee/internal/config"
)

// ConnectorList returns all connectors, optionally filtered by tags.
func (p *Platform) ConnectorList(tags []string) ([]api.ConnectorInfo, error) {
	cfg, err := p.loadConfig()
	if err != nil {
		return nil, err
	}

	var result []api.ConnectorInfo
	for name, c := range cfg.Connectors {
		if len(tags) > 0 && !hasAnyTag(c.Tags, tags) {
			continue
		}
		result = append(result, connectorToInfo(name, c))
	}
	return result, nil
}

// ConnectorGet returns a single connector by name.
func (p *Platform) ConnectorGet(name string) (*api.ConnectorInfo, error) {
	cfg, err := p.loadConfig()
	if err != nil {
		return nil, err
	}
	c, ok := cfg.Connectors[name]
	if !ok {
		return nil, NotFound(fmt.Sprintf("connector %q not found", name))
	}
	info := connectorToInfo(name, c)
	return &info, nil
}

// ConnectorCreate adds a new connector to angee.yaml and optionally stores a credential.
func (p *Platform) ConnectorCreate(ctx context.Context, req api.ConnectorCreateRequest) (*api.ConnectorInfo, error) {
	if req.Name == "" {
		return nil, BadRequest("name is required")
	}
	if req.Provider == "" {
		return nil, BadRequest("provider is required")
	}
	if req.Type == "" {
		return nil, BadRequest("type is required")
	}

	cfg, err := p.loadConfig()
	if err != nil {
		return nil, err
	}
	if cfg.Connectors == nil {
		cfg.Connectors = make(map[string]config.ConnectorSpec)
	}
	if _, exists := cfg.Connectors[req.Name]; exists {
		return nil, Conflict(fmt.Sprintf("connector %q already exists", req.Name))
	}

	spec := config.ConnectorSpec{
		Provider:    req.Provider,
		Type:        req.Type,
		Description: req.Description,
		Tags:        req.Tags,
		Metadata:    req.Metadata,
		Env:         req.Env,
	}
	cfg.Connectors[req.Name] = spec

	if err := p.writeAndCommit(cfg, fmt.Sprintf("add connector: %s", req.Name)); err != nil {
		return nil, err
	}

	info := connectorToInfo(req.Name, spec)
	return &info, nil
}

// ConnectorUpdate modifies an existing connector's metadata/tags.
func (p *Platform) ConnectorUpdate(ctx context.Context, name string, req api.ConnectorUpdateRequest) (*api.ConnectorInfo, error) {
	cfg, err := p.loadConfig()
	if err != nil {
		return nil, err
	}
	c, ok := cfg.Connectors[name]
	if !ok {
		return nil, NotFound(fmt.Sprintf("connector %q not found", name))
	}

	if req.Description != nil {
		c.Description = *req.Description
	}
	if req.Tags != nil {
		c.Tags = req.Tags
	}
	if req.Metadata != nil {
		c.Metadata = req.Metadata
	}
	cfg.Connectors[name] = c

	if err := p.writeAndCommit(cfg, fmt.Sprintf("update connector: %s", name)); err != nil {
		return nil, err
	}

	info := connectorToInfo(name, c)
	return &info, nil
}

// ConnectorDelete removes a connector from angee.yaml.
func (p *Platform) ConnectorDelete(ctx context.Context, name string) error {
	cfg, err := p.loadConfig()
	if err != nil {
		return err
	}
	if _, ok := cfg.Connectors[name]; !ok {
		return NotFound(fmt.Sprintf("connector %q not found", name))
	}

	delete(cfg.Connectors, name)
	return p.writeAndCommit(cfg, fmt.Sprintf("remove connector: %s", name))
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func connectorToInfo(name string, c config.ConnectorSpec) api.ConnectorInfo {
	return api.ConnectorInfo{
		Name:        name,
		Provider:    c.Provider,
		Type:        c.Type,
		Description: c.Description,
		Tags:        c.Tags,
		Metadata:    c.Metadata,
		Connected:   false, // TODO: check credential backend
	}
}

func hasAnyTag(have, want []string) bool {
	set := make(map[string]struct{}, len(have))
	for _, t := range have {
		set[t] = struct{}{}
	}
	for _, t := range want {
		if _, ok := set[t]; ok {
			return true
		}
	}
	return false
}
