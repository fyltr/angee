package cli

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
	"time"

	"github.com/spf13/cobra"
)

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Start the Angee platform",
	Long: `Start the Angee platform (operator, Django control plane, postgres, redis, traefik).

This starts the system stack. User services and agents defined in angee.yaml
are started automatically by the operator.

Example:
  angee up`,
	RunE: runUp,
}

func runUp(cmd *cobra.Command, args []string) error {
	path := resolveRoot()

	// Verify ANGEE_ROOT exists
	if _, err := os.Stat(filepath.Join(path, "angee.yaml")); os.IsNotExist(err) {
		return fmt.Errorf("ANGEE_ROOT not initialized at %s — run 'angee init' first", path)
	}

	fmt.Printf("\n\033[1mangee up\033[0m\n\n")

	// Write system compose file to .system/
	systemDir := filepath.Join(path, ".system")
	if err := os.MkdirAll(systemDir, 0755); err != nil {
		return fmt.Errorf("creating .system dir: %w", err)
	}

	systemComposePath := filepath.Join(systemDir, "docker-compose.yml")
	if err := writeSystemCompose(systemComposePath, path); err != nil {
		return fmt.Errorf("writing system compose: %w", err)
	}

	// Check if operator is already running
	if isOperatorRunning() {
		printInfo("Operator already running")
	} else {
		printInfo("Starting system stack...")
		if err := runDockerCompose(systemComposePath, path, "up", "-d", "--remove-orphans"); err != nil {
			return fmt.Errorf("starting system stack: %w", err)
		}

		// Wait for operator
		printInfo("Waiting for operator...")
		if err := waitForOperator(30 * time.Second); err != nil {
			return fmt.Errorf("operator did not start: %w", err)
		}
		printSuccess("Operator started (port 9000)")
	}

	// Trigger initial deploy via operator
	printInfo("Deploying angee.yaml...")
	if err := triggerDeploy(); err != nil {
		printError(fmt.Sprintf("deploy failed: %s (run 'angee logs' to debug)", err))
	} else {
		printSuccess("Services deployed")
	}

	fmt.Printf("\n\033[1mPlatform ready:\033[0m\n\n")
	printInfo("UI        →  \033[4mhttp://localhost:3333\033[0m")
	printInfo("API       →  \033[4mhttp://localhost:8000/api\033[0m")
	printInfo("Operator  →  \033[4mhttp://localhost:9000\033[0m")
	fmt.Println()
	printInfo("angee ls          View agents and services")
	printInfo("angee admin       Chat with admin agent")
	printInfo("angee develop     Chat with developer agent")
	fmt.Println()

	return nil
}

func isOperatorRunning() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", resolveOperator()+"/health", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

func waitForOperator(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if isOperatorRunning() {
			return nil
		}
		time.Sleep(1 * time.Second)
		fmt.Print(".")
	}
	fmt.Println()
	return fmt.Errorf("timeout waiting for operator after %s", timeout)
}

func triggerDeploy() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", resolveOperator()+"/deploy", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("operator returned %d", resp.StatusCode)
	}
	return nil
}

func runDockerCompose(composePath, projectDir string, args ...string) error {
	cmdArgs := []string{
		"compose",
		"--project-name", "angee-system",
		"--file", composePath,
		"--project-directory", projectDir,
	}
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.Command("docker", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func writeSystemCompose(path, angeeRoot string) error {
	tmpl := template.Must(template.New("system").Parse(systemComposeTemplate))
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]string{
		"AngeeRoot": angeeRoot,
	}); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0644)
}

// systemComposeTemplate is the system stack (operator + Django + deps).
// This is NOT committed to ANGEE_ROOT — it's the bootstrap layer.
const systemComposeTemplate = `# Angee system stack — managed by CLI, do not edit
# Source: embedded in angee CLI binary
name: angee-system

services:
  operator:
    image: ghcr.io/fyltr/angee-operator:latest
    restart: always
    ports:
      - "9000:9000"
    volumes:
      - {{ .AngeeRoot }}:/angee-root
      - /var/run/docker.sock:/var/run/docker.sock
    environment:
      - ANGEE_ROOT=/angee-root
      - DJANGO_URL=http://django:8000
    healthcheck:
      test: ["CMD", "wget", "-q", "-O", "-", "http://localhost:9000/health"]
      interval: 10s
      timeout: 5s
      retries: 10
      start_period: 5s

  django:
    image: ghcr.io/fyltr/angee-django:latest
    restart: always
    ports:
      - "8000:8000"
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    environment:
      - DATABASE_URL=postgresql://angee:angee@postgres:5432/angee
      - REDIS_URL=redis://redis:6379/0
      - ANGEE_OPERATOR_URL=http://operator:9000
      - SECRET_KEY=change-me-in-production
    volumes:
      - django-media:/app/media

  postgres:
    image: pgvector/pgvector:pg16
    restart: always
    environment:
      - POSTGRES_USER=angee
      - POSTGRES_PASSWORD=angee
      - POSTGRES_DB=angee
    volumes:
      - postgres-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U angee"]
      interval: 5s
      timeout: 5s
      retries: 10

  redis:
    image: redis:7-alpine
    restart: always
    volumes:
      - redis-data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 5s
      retries: 5

  traefik:
    image: traefik:v3
    restart: always
    ports:
      - "3333:80"
      - "443:443"
    command:
      - --providers.docker=true
      - --providers.docker.exposedbydefault=false
      - --entrypoints.web.address=:80
      - --entrypoints.websecure.address=:443
      - --api.dashboard=true
      - --api.insecure=true
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - traefik-certs:/certs

volumes:
  postgres-data:
  redis-data:
  django-media:
  traefik-certs:

networks:
  default:
    name: angee-system
`
