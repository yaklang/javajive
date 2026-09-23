package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// A checkcast in one conditional arm becomes a stack-only temporary in bytecode.
// The merge must keep the cast in that selected arm instead of hoisting its value
// as Object/null, and it must preserve the other arm's lazy evaluation.
func TestTernaryCheckcastMergeRoundTrip(t *testing.T) {
	javac, err := exec.LookPath("javac")
	if err != nil {
		t.Skip("javac is not installed")
	}
	java, err := exec.LookPath("java")
	if err != nil {
		t.Skip("java is not installed")
	}

	root := t.TempDir()
	originalDir := filepath.Join(root, "original")
	rebuiltDir := filepath.Join(root, "rebuilt")
	if err := os.MkdirAll(originalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(rebuiltDir, 0o755); err != nil {
		t.Fatal(err)
	}

	seed := `public class TernaryCheckcastMergeSeed {
	private final String value;
	private static int fallbackCalls;
	private static String fallback() { fallbackCalls++; return "fallback"; }
	public TernaryCheckcastMergeSeed(Object input) {
		this.value = input instanceof String ? (String) input : fallback();
	}
	public String value() { return value; }
	public static int fallbackCalls() { return fallbackCalls; }
}
`
	seedPath := filepath.Join(root, "TernaryCheckcastMergeSeed.java")
	if err := os.WriteFile(seedPath, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	javacBaseArgs := []string{"-J-Duser.language=en", "-J-Duser.country=US", "--release", "8", "-g:none"}
	if out, err := exec.Command(javac, append(append([]string{}, javacBaseArgs...), "-d", originalDir, seedPath)...).CombinedOutput(); err != nil {
		t.Fatalf("compile seed: %v\n%s", err, out)
	}
	raw, err := os.ReadFile(filepath.Join(originalDir, "TernaryCheckcastMergeSeed.class"))
	if err != nil {
		t.Fatal(err)
	}
	decompiled, err := Decompile(raw)
	if err != nil {
		t.Fatalf("decompile seed: %v", err)
	}
	decompiledPath := filepath.Join(root, "TernaryCheckcastMergeSeed.java")
	if err := os.WriteFile(decompiledPath, []byte(decompiled), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := `public class TernaryCheckcastMergeRunner {
	public static void main(String[] args) {
		if (!"selected".equals(new TernaryCheckcastMergeSeed("selected").value())) throw new AssertionError("true arm value");
		if (TernaryCheckcastMergeSeed.fallbackCalls() != 0) throw new AssertionError("false arm ran eagerly");
		if (!"fallback".equals(new TernaryCheckcastMergeSeed(new Object()).value())) throw new AssertionError("false arm value");
		if (TernaryCheckcastMergeSeed.fallbackCalls() != 1) throw new AssertionError("false arm call count");
	}
}
`
	runnerPath := filepath.Join(root, "TernaryCheckcastMergeRunner.java")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o644); err != nil {
		t.Fatal(err)
	}
	compileArgs := append(append([]string{}, javacBaseArgs...), "-cp", originalDir, "-d", rebuiltDir, decompiledPath, runnerPath)
	if out, err := exec.Command(javac, compileArgs...).CombinedOutput(); err != nil {
		t.Fatalf("compile decompiled seed: %v\n%s\n%s", err, out, decompiled)
	}
	if out, err := exec.Command(java, "-cp", rebuiltDir+string(os.PathListSeparator)+originalDir, "TernaryCheckcastMergeRunner").CombinedOutput(); err != nil {
		t.Fatalf("run round-trip seed: %v\n%s", err, out)
	}
}
