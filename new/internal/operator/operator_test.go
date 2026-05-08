package operator

import "testing"

func TestNewServerRequiresTokenForNonLoopbackBind(t *testing.T) {
	_, err := NewServer(Config{Root: t.TempDir(), Bind: "0.0.0.0", Port: 9000})
	if err == nil {
		t.Fatal("NewServer() error = nil, want token requirement")
	}
}
