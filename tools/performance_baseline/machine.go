package performance_baseline

import (
	"bytes"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func collectMachine() MachineInfo {
	host, _ := os.Hostname()
	uname := runCmd("uname", "-a")
	cpu := cpuBrand()
	return MachineInfo{
		Hostname:    host,
		OS:          runtime.GOOS,
		Arch:        runtime.GOARCH,
		NumCPU:      runtime.NumCPU(),
		GOMAXPROCS:  runtime.GOMAXPROCS(0),
		Uname:       strings.TrimSpace(uname),
		CPUBrand:    cpu,
		MachineNote: "Machine/OS/CPU differences are expected. Replay requires the same input family hashes and stats schema, not identical ns/op.",
	}
}

func collectToolchain() ToolchainInfo {
	return ToolchainInfo{
		GoVersion:    runtime.Version(),
		Compiler:     runtime.Compiler,
		GOTOOLCHAIN:  getenvDefault("GOTOOLCHAIN", "local"),
		CGOEnabled:   getenvDefault("CGO_ENABLED", "unknown"),
		Ldflags:      os.Getenv("T32_LDFLAGS"),
		GOOS:         runtime.GOOS,
		GOARCH:       runtime.GOARCH,
		JavacVersion: firstLine(runCmd("javac", "-version")),
		JavaVersion:  firstLine(runCmd("java", "-version")),
	}
}

func getenvDefault(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func runCmd(name string, args ...string) string {
	cmd := exec.Command(name, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	_ = cmd.Run()
	return buf.String()
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

func cpuBrand() string {
	if runtime.GOOS == "darwin" {
		return strings.TrimSpace(runCmd("sysctl", "-n", "machdep.cpu.brand_string"))
	}
	return ""
}

func gitSHA(repo string) (sha string, dirty bool) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		return "", true
	}
	sha = strings.TrimSpace(string(out))
	st := exec.Command("git", "status", "--porcelain")
	st.Dir = repo
	so, _ := st.Output()
	return sha, len(bytes.TrimSpace(so)) > 0
}
