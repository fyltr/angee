package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fyltr/angee/internal/copierx"
	"github.com/fyltr/angee/internal/manifest"
)

func (p *Platform) StackInit(ctx context.Context, template string, targetPath string, inputs map[string]string) error {
	if template == "" {
		return fmt.Errorf("stack template is required")
	}
	if targetPath == "" {
		targetPath = p.root
	}
	if !filepath.IsAbs(targetPath) {
		targetPath = filepath.Join(p.root, targetPath)
	}
	templatePath, _, err := p.resolveTemplate(template, "stack")
	if err != nil {
		return err
	}
	if _, err := copierx.ValidateMetadata(templatePath, "stack"); err != nil {
		return err
	}
	if err := os.MkdirAll(targetPath, 0o755); err != nil {
		return err
	}
	if err := (copierx.LocalRenderer{}).Copy(ctx, copierx.CopyRequest{Template: templatePath, Dest: targetPath, Inputs: copierx.Inputs(inputs)}); err != nil {
		return err
	}
	preparedRoot := targetPath
	if _, err := os.Stat(manifest.Path(preparedRoot)); err != nil {
		if angeeRoot, ok := inputs["ANGEE_ROOT"]; ok && angeeRoot != "" {
			candidate := manifest.ResolvePath(targetPath, angeeRoot)
			if _, statErr := os.Stat(manifest.Path(candidate)); statErr == nil {
				preparedRoot = candidate
			}
		} else {
			candidate := filepath.Join(targetPath, ".angee")
			if _, statErr := os.Stat(manifest.Path(candidate)); statErr == nil {
				preparedRoot = candidate
			}
		}
	}
	initialized, err := New(preparedRoot)
	if err != nil {
		return err
	}
	_, err = initialized.StackPrepare(ctx)
	return err
}

func (p *Platform) StackUpdate(ctx context.Context) error {
	_, err := p.StackPrepare(ctx)
	return err
}

func (p *Platform) StackDestroy(ctx context.Context, purge bool) error {
	if err := p.StackDown(ctx); err != nil {
		return err
	}
	for _, name := range []string{"docker-compose.yaml", "process-compose.yaml"} {
		if err := os.Remove(filepath.Join(p.root, name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if purge {
		for _, name := range []string{"workspaces", "sources", "volumes", "run"} {
			if err := os.RemoveAll(filepath.Join(p.root, name)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *Platform) EmptyStack(name string) *manifest.Stack {
	return &manifest.Stack{Version: manifest.VersionCurrent, Kind: manifest.KindStack, Name: name}
}
