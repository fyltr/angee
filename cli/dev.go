package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/fyltr/angee/internal/dev"
	"github.com/fyltr/angee/internal/projmode"
	"github.com/spf13/cobra"
)

var (
	devOnly       []string // --only=watcher,runtime,frontend,...
	devExcept     []string // --except=frontend,celery
	devNoWatch    bool     // --no-watch (alias --except=build)
	devNoFrontend bool     // --no-frontend (alias --except=frontend)
	devRuntime    string   // --runtime=django-angee  (override manifest)
	devUI         string   // --ui=lines|panes
)

var devCmd = &cobra.Command{
	Use:   "dev [flags]",
	Short: "Run the dev loop: build watcher + runtime dev server + frontend + extras",
	Long: `Run the development loop for an Angee consumer project.

Walks parents to find .angee/project.yaml, picks the runtime adapter from
its 'runtime:' field, and starts (in order):

  1. The build watcher    (e.g. uv run python manage.py angee build --watch)
  2. The runtime dev server (e.g. runserver)
  3. The frontend dev server (e.g. pnpm dev)
  4. Any [tool.angee.dev.processes.*] extras (e.g. docker compose up postgres redis)

Output is line-prefixed with stable per-name colours (default --ui=lines).
Use --ui=panes for a tab-per-child TUI (Phase 6).

Examples:
  angee dev                          # everything
  angee dev --no-frontend            # backend-only
  angee dev --only=runtime,frontend  # skip the watcher
  angee dev --ui=panes               # full-screen TUI`,
	RunE: runDev,
}

func init() {
	devCmd.Flags().StringSliceVar(&devOnly, "only", nil,
		"Run only these children (comma-separated; mutex with --except)")
	devCmd.Flags().StringSliceVar(&devExcept, "except", nil,
		"Run all children except these (comma-separated)")
	devCmd.Flags().BoolVar(&devNoWatch, "no-watch", false,
		"Skip the build watcher (alias --except=build)")
	devCmd.Flags().BoolVar(&devNoFrontend, "no-frontend", false,
		"Skip the frontend dev server (alias --except=frontend)")
	devCmd.Flags().StringVar(&devRuntime, "runtime", "",
		"Pin the runtime adapter (overrides manifest's runtime: field)")
	devCmd.Flags().StringVar(&devUI, "ui", "lines",
		"Output mode: lines | panes")
}

func runDev(cmd *cobra.Command, args []string) error {
	projectRoot := projmode.FindProjectRoot("")
	if projectRoot == "" {
		return errors.New("angee dev: no .angee/project.yaml found in this or any parent directory" +
			" (see `angee --help` for compose-mode commands)")
	}
	manifest, err := projmode.LoadManifest(projectRoot)
	if err != nil {
		return fmt.Errorf("angee dev: %w", err)
	}
	if devRuntime != "" {
		manifest.Runtime = devRuntime
	}
	adapter, err := pickAdapter(manifest.Runtime)
	if err != nil {
		return fmt.Errorf("angee dev: %w", err)
	}

	uvProject := ""
	if manifest.Django != nil && manifest.Django.UV != nil {
		uvProject = manifest.Django.UV.Project
	}
	py, err := projmode.ResolvePython(projectRoot, uvProject)
	if err != nil {
		return fmt.Errorf("angee dev: %w", err)
	}

	pyProject, err := projmode.LoadPyProjectAngeeDev(projectRoot)
	if err != nil {
		return fmt.Errorf("angee dev: %w", err)
	}

	ctx := projmode.Ctx{
		ProjectRoot: projectRoot,
		Manifest:    manifest,
		PyProject:   pyProject,
		Python:      py,
	}
	plan := dev.BuildPlan(adapter, ctx)
	opts := dev.PlanOptions{
		Only:       devOnly,
		Except:     devExcept,
		NoWatch:    devNoWatch,
		NoFrontend: devNoFrontend,
	}
	if err := plan.ApplyFilters(opts); err != nil {
		return fmt.Errorf("angee dev: %w", err)
	}

	sink, err := pickSink(devUI)
	if err != nil {
		return fmt.Errorf("angee dev: %w", err)
	}
	sink.SystemLine(
		"project: %s  runtime: %s  python: %s",
		projectRoot, manifest.Runtime, py.Strategy,
	)

	// Pane-mode sink runs a bubbletea program in its own goroutine.
	// When the user hits q/Ctrl-C in the TUI, forward to ourselves as
	// SIGINT so the orchestrator's signal handler shuts children down
	// cleanly. After Run returns, dismiss the TUI.
	if pane, ok := sink.(*dev.PaneSink); ok {
		go func() {
			<-pane.Done()
			if proc, err := os.FindProcess(os.Getpid()); err == nil {
				_ = proc.Signal(syscall.SIGINT)
			}
		}()
		defer func() {
			pane.Quit()
			pane.Wait()
		}()
	}

	return dev.Run(context.Background(), plan, sink)
}

// pickSink chooses an output strategy. Lines is the universally-available
// default; panes is a bubbletea TUI that requires a real TTY. When --ui
// is unset and stdout looks like an IDE-embedded terminal (VSCode /
// JetBrains) we stay on lines.
func pickSink(mode string) (dev.Sink, error) {
	switch strings.ToLower(mode) {
	case "", "lines":
		return dev.NewLineSink(os.Stdout, false), nil
	case "panes":
		return dev.NewPaneSink(), nil
	default:
		return nil, fmt.Errorf("--ui=%q: must be 'lines' or 'panes'", mode)
	}
}
