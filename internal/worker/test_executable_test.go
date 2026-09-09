package worker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// buildTestExecutable creates a real executable for lifecycle tests on both
// Unix and Windows. It avoids pretending that shell text is a .exe file.
func buildTestExecutable(t *testing.T, dir string, exitCode int) string {
	t.Helper()
	source := filepath.Join(dir, fmt.Sprintf("helper-%d.go", exitCode))
	output := filepath.Join(dir, fmt.Sprintf("helper-%d", exitCode))
	if runtime.GOOS == "windows" {
		output += ".exe"
	}
	program := fmt.Sprintf("package main\nimport \"os\"\nfunc main() { os.Exit(%d) }\n", exitCode)
	if err := os.WriteFile(source, []byte(program), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-o", output, source)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build test executable: %v\n%s", err, output)
	}
	return output
}
