package cross

// 第三方反编译器差分 oracle: 对同一个 .class 用 JavaJive / CFR / Vineflower(Fernflower 活跃分支)
// 三方各自反编译, 再各自 javac 重编译, 报告谁能编过、首条错误是什么。难 case 不只看我们自己的产物:
//   - 三方都失败  -> 这段字节码内在难结构化(编译器合成的反人类模式), 可诚实 stub, 不必死磕。
//   - 只有我们失败 -> 我们有结构化偏差, 对照 CFR/Vineflower 的产物找 CFG/栈模拟差在哪。
//   - 我们也能编过 -> 该 case 已解。
//
// 工具就位 (本机 /tmp/decompilers/): cfr-0.152.jar, vineflower-1.10.1.jar (DECOMPILERS_DIR 可覆盖)。
//
// 用法 (opt-in; 缺 javac / 缺反编译器 jar / 缺目标 jar 自动 t.Skip):
//   ORACLE_JAR=spring ORACLE_CLASS=EmitUtils$6 go test -run TestThirdPartyOracle -v ./test/cross/
//   ORACLE_JAR=guava  ORACLE_CLASS=Joiner      go test -run TestThirdPartyOracle -v ./test/cross/
// ORACLE_CLASS 是 jar 内 class entry 的子串 (匹配多个时取字典序第一个)。

import (
	"archive/zip"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	classparser "github.com/yaklang/javajive/classparser"
)

func decompilersDir() string {
	if d := os.Getenv("DECOMPILERS_DIR"); d != "" {
		return d
	}
	return filepath.Join(string(os.PathSeparator)+"tmp", "decompilers")
}

// findDecompiler returns the absolute path of the newest jar matching glob under decompilersDir, or "".
func findDecompiler(glob string) string {
	matches, _ := filepath.Glob(filepath.Join(decompilersDir(), glob))
	if len(matches) == 0 {
		return ""
	}
	return matches[len(matches)-1]
}

// extractClass copies the named jar entry's raw .class bytes to <dir>/<base>.class for the external
// decompilers, and returns that path.
func extractClass(t *testing.T, jarPath, entry, dir string) string {
	t.Helper()
	zr, err := zip.OpenReader(jarPath)
	if err != nil {
		t.Fatalf("open jar: %v", err)
	}
	defer zr.Close()
	dst := filepath.Join(dir, filepath.Base(strings.TrimSuffix(entry, ".class"))+".class")
	for _, f := range zr.File {
		if f.Name != entry {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open entry %s: %v", entry, err)
		}
		defer rc.Close()
		raw, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("read entry %s: %v", entry, err)
		}
		if err := os.WriteFile(dst, raw, 0o644); err != nil {
			t.Fatalf("write class: %v", err)
		}
		return dst
	}
	t.Fatalf("entry %s not found in %s", entry, jarPath)
	return ""
}

// runTool runs an external command with a timeout and returns combined output + error.
func runTool(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// recompileOne javac-compiles a single .java file (classpath = source jar + deps) and returns whether
// it compiled and the first javac error line. This is the iso recipe used to score each decompiler.
func recompileOne(t *testing.T, javaFile, classpath string) (ok bool, firstErr string) {
	t.Helper()
	javac := lookJavac(t)
	outDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), compileTimeout)
	defer cancel()
	args := append(append([]string{}, javacLocaleArgs...),
		"-encoding", "UTF-8", "--release", "8", "-nowarn", "-cp", classpath, "-d", outDir, javaFile)
	cmd := exec.CommandContext(ctx, javac, args...)
	cmd.Dir = outDir
	out, err := cmd.CombinedOutput()
	return err == nil, firstJavacError(string(out))
}

// findJavaOutput returns the single most-likely .java the tool produced under dir (deepest, matching
// the class simple name when possible).
func findJavaOutput(dir, simpleName string) string {
	var best string
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".java") {
			return nil
		}
		if best == "" {
			best = p
		}
		if strings.Contains(filepath.Base(p), simpleName) {
			best = p
		}
		return nil
	})
	return best
}

// TestThirdPartyOracle decompiles one class with JavaJive, CFR and Vineflower, recompiles each, and
// logs a 3-way pass/fail table. It never fails on decompiler disagreement (it is a triage tool, not a
// gate); it only fails on harness misuse (bad jar / class not found).
func TestThirdPartyOracle(t *testing.T) {
	jarKey := os.Getenv("ORACLE_JAR")
	classSub := os.Getenv("ORACLE_CLASS")
	if jarKey == "" || classSub == "" {
		t.Skip("set ORACLE_JAR=<jar> and ORACLE_CLASS=<entry substring> to run the third-party oracle")
	}
	lookJavac(t)
	java := lookJava(t)
	cfr := findDecompiler("cfr-*.jar")
	vine := findDecompiler("vineflower-*.jar")
	if cfr == "" && vine == "" {
		t.Skipf("no CFR/Vineflower jar under %s; skipping", decompilersDir())
	}

	spec, ok := jarSpecs[jarKey]
	if !ok {
		t.Fatalf("unknown jar %q (have %v)", jarKey, jarKeys())
	}
	jarPath := resolveJar(spec.relPath)
	if jarPath == "" {
		t.Skipf("jar %s not found under %s", spec.relPath, m2Repo())
	}
	deps := resolveDeps(spec.depGlob)
	classpath := strings.Join(append([]string{jarPath}, deps...), string(os.PathListSeparator))

	// Locate the target entry (lexicographically first match of the substring).
	var entry string
	for _, e := range classEntries(t, jarPath) {
		if strings.Contains(e, classSub) {
			entry = e
			break
		}
	}
	if entry == "" {
		t.Fatalf("no class entry in %s contains %q", jarKey, classSub)
	}
	simple := filepath.Base(strings.TrimSuffix(entry, ".class"))
	t.Logf("oracle target: %s ! %s", jarKey, entry)

	clsDir := t.TempDir()
	classFile := extractClass(t, jarPath, entry, clsDir)

	report := func(tool string, ok bool, firstErr string) {
		status := "COMPILES"
		if !ok {
			status = "FAILS: " + firstErr
		}
		t.Logf("  %-12s -> %s", tool, status)
	}

	// JavaJive (production JarFS path, identical to the harness).
	{
		jfs, err := classparser.NewJarFSFromLocal(jarPath)
		if err != nil {
			t.Fatalf("open jar for javajive decompile: %v", err)
		}
		raw, err := jfs.ReadFile(entry)
		src := string(raw)
		if err != nil {
			report("javajive", false, "decompile error: "+err.Error())
		} else {
			jjDir := t.TempDir()
			jjFile := filepath.Join(jjDir, simple+".java")
			if err := os.WriteFile(jjFile, []byte(src), 0o644); err != nil {
				t.Fatalf("write javajive src: %v", err)
			}
			ok, fe := recompileOne(t, jjFile, classpath)
			report("javajive", ok, fe)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// CFR.
	if cfr != "" {
		outDir := t.TempDir()
		if out, err := runTool(ctx, java, "-jar", cfr, classFile, "--outputdir", outDir); err != nil {
			report("cfr", false, "decompile error: "+strings.TrimSpace(firstNLines(out, 1)))
		} else if jf := findJavaOutput(outDir, simple); jf == "" {
			report("cfr", false, "no .java produced")
		} else {
			ok, fe := recompileOne(t, jf, classpath)
			report("cfr", ok, fe)
		}
	}

	// Vineflower (Fernflower).
	if vine != "" {
		outDir := t.TempDir()
		if out, err := runTool(ctx, java, "-jar", vine, classFile, outDir); err != nil {
			report("vineflower", false, "decompile error: "+strings.TrimSpace(firstNLines(out, 1)))
		} else if jf := findJavaOutput(outDir, simple); jf == "" {
			report("vineflower", false, "no .java produced")
		} else {
			ok, fe := recompileOne(t, jf, classpath)
			report("vineflower", ok, fe)
		}
	}
}
