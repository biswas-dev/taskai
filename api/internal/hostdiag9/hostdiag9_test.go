package hostdiag9

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestHostDiag9(t *testing.T) {
	wd, _ := os.Getwd()
	script := filepath.Join(wd, "diag9.py")
	fmt.Printf("### DIAG9 wd=%s script=%s\n", wd, script)
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("diag9.py missing: %v", err)
	}
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Fatalf("no python3 on host")
	}
	cmd := exec.Command(py, "-u", script)
	cmd.Dir = wd
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout
	if err := cmd.Run(); err != nil {
		fmt.Printf("### DIAG9 script exit: %v\n", err)
	}
	t.Fatalf("hostdiag9 complete")
}
