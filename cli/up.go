package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/fyltr/angee/internal/compiler"
	"github.com/fyltr/angee/internal/config"
	"github.com/fyltr/angee/internal/credentials/openbao"
	"github.com/fyltr/angee/internal/root"
	"github.com/fyltr/angee/internal/secrets"
	"github.com/spf13/cobra"
)

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Start the Angee platform",
	Long: `Compile angee.yaml → docker-compose.yaml, render agent config files,
and run docker compose up. Changed services are automatically recreated.

Example:
  angee up`,
	RunE: runUp,
}

func runUp(cmd *cobra.Command, args []string) error {
	path := resolveRoot()

	// Load angee.yaml
	cfg, err := config.Load(filepath.Join(path, "angee.yaml"))
	if err != nil {
		return fmt.Errorf("loading angee.yaml: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	projectName := cfg.Name
	if projectName == "" {
		projectName = "angee"
	}

	r, err := root.Open(path)
	if err != nil {
		return fmt.Errorf("opening ANGEE_ROOT: %w", err)
	}

	fmt.Printf("\n\033[1mangee up\033[0m\n\n")

	// Ensure agent directories + render config file templates
	for agentName, agent := range cfg.Agents {
		if err := r.EnsureAgentDir(agentName); err != nil {
			return fmt.Errorf("creating agent dir %s: %w", agentName, err)
		}
		if err := compiler.RenderAgentFiles(path, r.AgentDir(agentName), agent, cfg.MCPServers); err != nil {
			return fmt.Errorf("agent files for %s: %w", agentName, err)
		}
	}
	printSuccess("Rendered agent config files")

	// Compile angee.yaml → docker-compose.yaml
	opCfg, err := r.LoadOperatorConfig()
	if err != nil {
		return fmt.Errorf("loading operator config: %w", err)
	}
	comp := compiler.New(path, opCfg.Docker.Network)
	// Auto-inject operator API key into operator-role agents
	if opCfg.APIKey != "" {
		comp.APIKey = opCfg.APIKey
	} else if key := os.Getenv("ANGEE_API_KEY"); key != "" {
		comp.APIKey = key
	}
	cf, err := comp.Compile(cfg)
	if err != nil {
		return fmt.Errorf("compiling: %w", err)
	}
	composePath := filepath.Join(path, "docker-compose.yaml")
	if err := compiler.Write(cf, composePath); err != nil {
		return fmt.Errorf("writing docker-compose.yaml: %w", err)
	}
	printSuccess("Compiled docker-compose.yaml")

	// Two-phase startup when OpenBao is configured:
	// 1. Start infrastructure services (operator, traefik, openbao)
	// 2. Seed OpenBao from .env, pull .env from OpenBao
	// 3. Start all remaining services
	if hasOpenBaoBackend(cfg) {
		infraServices := identifyInfraServices(cfg)
		if len(infraServices) > 0 {
			printInfo("Phase 1: Starting infrastructure services...")
			infraArgs := append([]string{"up", "-d"}, infraServices...)
			if err := runDockerCompose(composePath, path, projectName, infraArgs...); err != nil {
				fmt.Printf("  \033[33m!\033[0m  Some infrastructure containers failed to start\n")
			} else {
				printSuccess(fmt.Sprintf("Infrastructure started (%s)", strings.Join(infraServices, ", ")))
			}

			// Wait for OpenBao, seed secrets, pull .env
			envPath := filepath.Join(path, ".env")
			if bao, err := waitForOpenBao(cfg, 60*time.Second); err != nil {
				printInfo(fmt.Sprintf("OpenBao not ready (%v) — using .env as-is", err))
			} else {
				seeded, err := secrets.SeedOpenBao(context.Background(), bao, envPath)
				if err != nil {
					printInfo(fmt.Sprintf("Seed warning: %v", err))
				} else if seeded > 0 {
					printSuccess(fmt.Sprintf("Seeded %d secret(s) into OpenBao", seeded))
				}

				count, err := secrets.PullToEnvFile(context.Background(), bao, envPath)
				if err != nil {
					printInfo(fmt.Sprintf("Pull warning: %v", err))
				} else if count > 0 {
					printSuccess(fmt.Sprintf("Regenerated .env from OpenBao (%d secret(s))", count))
				}
			}
		}
	}

	// Start all services (infrastructure already running = no-op for those)
	printInfo("Starting stack...")
	if err := runDockerCompose(composePath, path, projectName, "up", "-d", "--build", "--remove-orphans"); err != nil {
		fmt.Printf("  \033[33m!\033[0m  Some containers failed to start (run 'angee logs' to investigate)\n")
	} else {
		printSuccess("Stack started")
	}

	printPlatformReady()
	return nil
}

// hasOpenBaoBackend returns true if the config declares an openbao secrets backend.
func hasOpenBaoBackend(cfg *config.AngeeConfig) bool {
	return cfg.SecretsBackend != nil && cfg.SecretsBackend.Type == "openbao" && cfg.SecretsBackend.OpenBao != nil
}

// identifyInfraServices returns the names of services marked infrastructure: true.
func identifyInfraServices(cfg *config.AngeeConfig) []string {
	var names []string
	for name, svc := range cfg.Services {
		if svc.Infrastructure {
			names = append(names, name)
		}
	}
	return names
}

// waitForOpenBao creates an OpenBao backend using the host-reachable address
// and waits for it to become ready.
func waitForOpenBao(cfg *config.AngeeConfig, timeout time.Duration) (*openbao.Backend, error) {
	obCfg := resolveOpenBaoHostAddress(cfg)
	env := cfg.Environment
	if env == "" {
		env = "dev"
	}

	// Ensure the token env var is set. In dev mode, the token is often
	// hardcoded in the operator/openbao service env block rather than in
	// the host environment. Look it up and export it.
	ensureBaoTokenEnv(cfg, obCfg)

	// Use NewFromEnv (no reachability check) since we'll WaitReady below.
	bao := openbao.NewFromEnv(obCfg, env)
	if bao == nil {
		return nil, fmt.Errorf("cannot create OpenBao client (token missing?)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := bao.WaitReady(ctx); err != nil {
		return nil, err
	}
	return bao, nil
}

// ensureBaoTokenEnv sets the BAO_TOKEN env var from angee.yaml service configs
// if it's not already set in the host environment. This handles the common dev
// pattern where BAO_TOKEN=dev-root-token is in the operator service env block.
func ensureBaoTokenEnv(cfg *config.AngeeConfig, obCfg *config.OpenBaoConfig) {
	tokenEnv := obCfg.Auth.TokenEnv
	if tokenEnv == "" {
		tokenEnv = "BAO_TOKEN"
	}
	if os.Getenv(tokenEnv) != "" {
		return // already set
	}

	// Search service env blocks for the token variable
	for _, svc := range cfg.Services {
		if val, ok := svc.Env[tokenEnv]; ok && val != "" {
			os.Setenv(tokenEnv, val)
			return
		}
	}
}

// resolveOpenBaoHostAddress returns a copy of the OpenBao config with the
// container-internal address replaced by a host-reachable one.
// "http://openbao:8200" → "http://localhost:8200"
func resolveOpenBaoHostAddress(cfg *config.AngeeConfig) *config.OpenBaoConfig {
	ob := *cfg.SecretsBackend.OpenBao // shallow copy
	addr := ob.Address
	// Replace container hostname with localhost for host-side access
	addr = strings.Replace(addr, "://openbao:", "://localhost:", 1)
	addr = strings.Replace(addr, "://openbao/", "://localhost/", 1)
	// Handle case where address is just "http://openbao:8200" (no trailing path)
	if strings.HasSuffix(addr, "://openbao") {
		addr = strings.Replace(addr, "://openbao", "://localhost", 1)
	}
	ob.Address = addr
	return &ob
}

func printPlatformReady() {
	fmt.Printf("\n\033[1mPlatform ready:\033[0m\n\n")
	printInfo("UI        →  \033[4mhttp://localhost:3333\033[0m")
	printInfo("API       →  \033[4mhttp://localhost:8000/api\033[0m")
	printInfo("Operator  →  \033[4mhttp://localhost:9000\033[0m")
	fmt.Println()
	printInfo("angee ls          View agents and services")
	printInfo("angee admin       Chat with admin agent")
	printInfo("angee develop     Chat with developer agent")
	fmt.Println()
}

func runDockerCompose(composePath, projectDir, projectName string, args ...string) error {
	cmdArgs := []string{
		"compose",
		"--project-name", projectName,
		"--file", composePath,
		"--project-directory", projectDir,
	}
	// Pass .env file if it exists
	envFile := filepath.Join(projectDir, ".env")
	if _, err := os.Stat(envFile); err == nil {
		cmdArgs = append(cmdArgs, "--env-file", envFile)
	}
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.Command("docker", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
