package service

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestSurfaceMatrixMentionsEveryExportedPlatformMethod(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	docPath := filepath.Join(repoRoot, "docs", "reference", "surfaces.md")
	data, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", docPath, err)
	}
	doc := string(data)

	platformType := reflect.TypeOf((*Platform)(nil))
	for i := 0; i < platformType.NumMethod(); i++ {
		name := platformType.Method(i).Name
		if !strings.Contains(doc, "| `"+name+"` |") {
			t.Fatalf("%s does not classify Platform.%s", docPath, name)
		}
	}
}
