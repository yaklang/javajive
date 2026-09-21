package performance_baseline

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// javac-generated families: original bytes must pass java -Xverify:all.
// Hand-assembled classfiles without StackMapTable are not accepted.

var (
	javacOnce  sync.Once
	javacCache map[string][]byte
)

func javaSourceLoop(pkg, name string, n int) string {
	return fmt.Sprintf(`package %s;
public class %s {
  public static int run() {
    int s = 0;
    for (int i = 0; i < %d; i++) {
      int t = 0;
      for (int j = 0; j <= i; j++) t += j;
      s += t;
    }
    return s;
  }
  public static void main(String[] a) { System.out.println(run()); }
}
`, pkg, name, n)
}

func javaSourceSlot(pkg, name string, n int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "package %s;\npublic class %s {\n  public static int run() {\n", pkg, name)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "    int v%d = %d;\n", i, i+1)
	}
	b.WriteString("    int s = 0;\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "    s += v%d;\n", i)
	}
	b.WriteString("    if ((s & 1) == 0) {\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "      s += v%d;\n", i)
	}
	b.WriteString("    }\n    return s;\n  }\n  public static void main(String[] a) { System.out.println(run()); }\n}\n")
	return b.String()
}

func javaSourceDeep(pkg, name string, n int) string {
	expr := "1"
	for i := 0; i < n; i++ {
		expr = "(" + expr + "+" + fmt.Sprintf("%d", i+1) + ")"
	}
	return fmt.Sprintf(`package %s;
public class %s {
  public static int run() { return %s; }
  public static void main(String[] a) { System.out.println(run()); }
}
`, pkg, name, expr)
}

func javaSourceHandlerDense(pkg, name string, n int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "package %s;\npublic class %s {\n  public static int run() {\n    int s = 0;\n", pkg, name)
	for i := 0; i < n; i++ {
		b.WriteString("    try {\n")
	}
	b.WriteString("      s += 1;\n      if (s < 0) throw new RuntimeException();\n")
	for i := n - 1; i >= 0; i-- {
		fmt.Fprintf(&b, "    } catch (RuntimeException e) { s += %d; }\n", i+1)
	}
	b.WriteString("    return s;\n  }\n  public static void main(String[] a) { System.out.println(run()); }\n}\n")
	return b.String()
}

func javaSourceHandlerSparse(pkg, name string, n int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "package %s;\npublic class %s {\n  public static int run() {\n    int s = 0;\n", pkg, name)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "    try { s += %d; if (s < 0) throw new RuntimeException(); } catch (RuntimeException e) { s += 1; }\n", i+1)
	}
	b.WriteString("    return s;\n  }\n  public static void main(String[] a) { System.out.println(run()); }\n}\n")
	return b.String()
}

func compileAndVerifyJava(internal, source string) ([]byte, error) {
	if _, err := exec.LookPath("javac"); err != nil {
		return nil, fmt.Errorf("infra_error: javac required to freeze T32 families: %w", err)
	}
	if _, err := exec.LookPath("java"); err != nil {
		return nil, fmt.Errorf("infra_error: java required to verify T32 families: %w", err)
	}
	dir, err := os.MkdirTemp("", "t32-javac-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	srcPath := filepath.Join(dir, filepath.FromSlash(internal)+".java")
	if err := os.MkdirAll(filepath.Dir(srcPath), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(srcPath, []byte(source), 0o644); err != nil {
		return nil, err
	}
	cmd := exec.Command("javac", "-proc:none", "-encoding", "UTF-8", "--release", "8", "-g:none", "-d", dir, srcPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("javac %s: %v: %s", internal, err, out)
	}
	classPath := filepath.Join(dir, filepath.FromSlash(internal)+".class")
	raw, err := os.ReadFile(classPath)
	if err != nil {
		return nil, err
	}
	javaName := strings.ReplaceAll(internal, "/", ".")
	vcmd := exec.Command("java", "-Xverify:all", "-cp", dir, javaName)
	vout, verr := vcmd.CombinedOutput()
	if verr != nil {
		return nil, fmt.Errorf("java -Xverify:all rejected %s (not accepted): %v: %s", internal, verr, vout)
	}
	return raw, nil
}

func verifyAllClassBytes(internal string, raw []byte) error {
	if _, err := exec.LookPath("java"); err != nil {
		return fmt.Errorf("infra_error: java required: %w", err)
	}
	dir, err := os.MkdirTemp("", "t32-verify-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	classPath := filepath.Join(dir, filepath.FromSlash(internal)+".class")
	if err := os.MkdirAll(filepath.Dir(classPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(classPath, raw, 0o644); err != nil {
		return err
	}
	javaName := strings.ReplaceAll(internal, "/", ".")
	cmd := exec.Command("java", "-Xverify:all", "-cp", dir, javaName)
	out, err := cmd.CombinedOutput()
	text := string(out)
	if strings.Contains(text, "VerifyError") || strings.Contains(text, "StackMapTable") {
		return fmt.Errorf("java -Xverify:all rejected %s (not accepted): %s", internal, text)
	}
	loaded := strings.Contains(text, "Main method") ||
		strings.Contains(text, "main method") ||
		strings.Contains(text, "找不到 main") ||
		strings.Contains(text, "Error: Main method")
	if err != nil && !loaded {
		return fmt.Errorf("java -Xverify:all %s: %v: %s", internal, err, text)
	}
	return nil
}

func mustJavacFamily(internal, source string) []byte {
	javacOnce.Do(func() { javacCache = map[string][]byte{} })
	if raw, ok := javacCache[internal]; ok {
		return raw
	}
	raw, err := compileAndVerifyJava(internal, source)
	if err != nil {
		panic(err)
	}
	javacCache[internal] = raw
	return raw
}
