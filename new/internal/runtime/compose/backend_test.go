package compose

import (
	"context"
	"reflect"
	"testing"

	"github.com/fyltr/angee/internal/runtime"
)

type recordingRunner struct {
	name string
	args []string
	out  []byte
}

func (r *recordingRunner) Run(_ context.Context, _ string, name string, args ...string) ([]byte, error) {
	r.name = name
	r.args = append([]string(nil), args...)
	return r.out, nil
}

func TestBackendUpCommand(t *testing.T) {
	runner := &recordingRunner{}
	backend := Backend{Runner: runner}
	err := backend.Up(context.Background(), runtime.Target{Root: "/stack", EnvFile: "/stack/.env", Services: []string{"web"}, Build: true})
	if err != nil {
		t.Fatalf("Up() error = %v", err)
	}
	want := []string{"compose", "-f", "/stack/docker-compose.yaml", "--env-file", "/stack/.env", "up", "-d", "--build", "web"}
	if runner.name != "docker" || !reflect.DeepEqual(runner.args, want) {
		t.Fatalf("command = %s %v, want docker %v", runner.name, runner.args, want)
	}
}

func TestParsePS(t *testing.T) {
	got := parsePS([]byte(`{"Service":"web","State":"running"}
{"Service":"db","State":"exited"}
`))
	if len(got) != 2 || got[0].Name != "web" || got[0].State != "running" {
		t.Fatalf("parsePS() = %#v", got)
	}
}
