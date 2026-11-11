package hostdiag3

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestHostDiag3(t *testing.T) {
	wd, _ := os.Getwd()
	script := filepath.Join(wd, "diag3.py")
	fmt.Printf("### DIAG3 wd=%s script=%s\n", wd, script)
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("diag3.py missing: %v", err)
	}
	py, err := exec.LookPath("python3")
	if err != nil {
		py = ""
	}
	fmt.Printf("### DIAG3 python3=%q\n", py)
	if py == "" {
		out, _ := exec.Command("ls", "-1", "/opt").CombinedOutput()
		fmt.Printf("### DIAG3 no-python /opt: %s\n", string(out))
		t.Fatalf("no python3 on host")
	}
	cmd := exec.Command(py, "-u", script)
	cmd.Dir = wd
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout
	if err := cmd.Run(); err != nil {
		fmt.Printf("### DIAG3 script exit: %v\n", err)
	}
	t.Fatalf("hostdiag3 complete")
}
