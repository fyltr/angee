package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fyltr/angee/api"
)

func ensureLocalOperator(rootPath string) error {
	if rootPath == "" {
		rootPath = resolveRoot()
	}
	rootPath = expandPath(rootPath)
	if ok, health := operatorHealthyForRoot(rootPath); ok {
		return nil
	} else if health != nil {
		return fmt.Errorf("operator at %s is already serving ANGEE_ROOT %s, not %s", resolveOperator(), health.Root, rootPath)
	}
	if explicitOperatorConfigured() {
		return fmt.Errorf("operator at %s is not running for ANGEE_ROOT %s", resolveOperator(), rootPath)
	}
	if err := spawnEmbeddedOperator(rootPath); err != nil {
		return operatorStartError(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if ok, _ := operatorHealthyForRoot(rootPath); ok {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("embedded operator did not become ready at %s", resolveOperator())
}

func explicitOperatorConfigured() bool {
	return operatorURL != "" || os.Getenv("ANGEE_OPERATOR_URL") != ""
}

func operatorHealthyForRoot(rootPath string) (bool, *api.HealthResponse) {
	req, err := newRequest("GET", resolveOperator()+"/health", nil)
	if err != nil {
		return false, nil
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return false, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return false, nil
	}
	var health api.HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return false, nil
	}
	if samePath(health.Root, rootPath) {
		return true, &health
	}
	return false, &health
}

func spawnEmbeddedOperator(rootPath string) error {
	if err := os.MkdirAll(filepath.Join(rootPath, "state"), 0755); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	port := operatorPortFromURL(resolveOperator())
	logPath := filepath.Join(rootPath, "state", "operator.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	cmd := exec.Command(exe,
		"--root", rootPath,
		"operator",
		"--bind", "127.0.0.1",
		"--port", strconv.Itoa(port),
	)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = append(os.Environ(),
		"ANGEE_ROOT="+rootPath,
		"ANGEE_OPERATOR_PORT="+strconv.Itoa(port),
		"ANGEE_BIND_ADDRESS=127.0.0.1",
	)
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return err
	}
	pidPath := filepath.Join(rootPath, "state", "operator.pid")
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(cmd.Process.Pid)+"\n"), 0644); err != nil {
		_ = logFile.Close()
		return err
	}
	return logFile.Close()
}

func operatorPortFromURL(raw string) int {
	u, err := url.Parse(raw)
	if err != nil {
		return 9000
	}
	if p := u.Port(); p != "" {
		if port, err := strconv.Atoi(p); err == nil && port > 0 {
			return port
		}
	}
	return 9000
}

func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	absA, errA := filepath.Abs(expandPath(a))
	absB, errB := filepath.Abs(expandPath(b))
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return strings.EqualFold(filepath.Clean(absA), filepath.Clean(absB))
}
