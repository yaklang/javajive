package adversarial

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type generatedRecordComponent struct {
	typ  string
	name string
}

func generatedRecordSources(name string, records [][]generatedRecordComponent, rng *rand.Rand) (string, []string) {
	var source strings.Builder
	source.WriteString("public class " + name + " {\n")
	names := make([]string, len(records))
	for i := range records {
		names[i] = fmt.Sprintf("R%d", i)
	}
	source.WriteString("    sealed interface Shape permits " + strings.Join(names, ", ") + " {}\n")
	var samples []string
	for i, components := range records {
		fields := make([]string, len(components))
		args := make([]string, len(components))
		for j, component := range components {
			fields[j] = component.typ + " " + component.name
			switch component.typ {
			case "int":
				args[j] = fmt.Sprint(rng.Intn(7) - 3)
			case "long":
				args[j] = fmt.Sprintf("%dL", rng.Intn(7)-3)
			case "double":
				args[j] = fmt.Sprintf("%d.5", rng.Intn(5)-2)
			default:
				args[j] = fmt.Sprintf("%q", fmt.Sprintf("s%d", rng.Intn(5)))
			}
		}
		source.WriteString(fmt.Sprintf("    record R%d(%s) implements Shape {}\n", i, strings.Join(fields, ", ")))
		samples = append(samples, fmt.Sprintf("R%d(%s)", i, strings.Join(args, ", ")))
	}
	source.WriteString("    static String describe(Shape s) {\n")
	source.WriteString("        return switch (s) {\n")
	for i, components := range records {
		pattern := make([]string, len(components))
		expr := fmt.Sprintf("%q", fmt.Sprintf("R%d", i))
		for j, component := range components {
			pattern[j] = component.typ + " " + component.name
			expr += " + \":\" + " + component.name
		}
		source.WriteString(fmt.Sprintf("            case R%d(%s) -> %s;\n", i, strings.Join(pattern, ", "), expr))
	}
	source.WriteString("        };\n")
	source.WriteString("    }\n")
	source.WriteString("    public static void main(String[] args) {\n")
	for _, sample := range samples {
		source.WriteString(fmt.Sprintf("        System.out.println(describe(new %s));\n", sample))
	}
	source.WriteString("    }\n")
	source.WriteString("}\n")
	return source.String(), samples
}

// Generate rotating record shapes (1-3 components, primitive and reference
// components) and round-trip each through decompile -> tree javac ->
// -Xverify:all -> behavior comparison. This exercises the T21 component parser
// beyond the single checked-in deconstruction fixture.
func TestAdversarialGeneratedRecordPatternsRoundTrip(t *testing.T) {
	if !envEnabled() {
		t.Skip("set JAVAJIVE_ADVERSARIAL_EVAL=1 to run the adversarial evaluation")
	}
	javac, err := exec.LookPath("javac")
	if err != nil {
		t.Skip("javac not found")
	}
	java, err := exec.LookPath("java")
	if err != nil {
		t.Skip("java not found")
	}
	if javacMajor(t, javac) < 21 {
		t.Skip("record deconstruction requires javac 21")
	}
	types := []string{"int", "long", "double"}
	for round := 0; round < 3; round++ {
		rng := rand.New(rand.NewSource(int64(0x51ee + round)))
		for caseIndex := 0; caseIndex < 3; caseIndex++ {
			name := fmt.Sprintf("GenRec%d_%d", round, caseIndex)
			recordCount := 1 + rng.Intn(3)
			records := make([][]generatedRecordComponent, recordCount)
			for i := range records {
				componentCount := 1 + rng.Intn(3)
				components := make([]generatedRecordComponent, componentCount)
				for j := range components {
					components[j] = generatedRecordComponent{
						typ:  types[rng.Intn(len(types))],
						name: string(rune('a' + j)),
					}
				}
				records[i] = components
			}
			source, _ := generatedRecordSources(name, records, rng)
			dir := compileConstructorFixture(t, javac, name, source)
			originalRun, err := exec.Command(java, "-Xverify:all", "-cp", dir, name).CombinedOutput()
			if err != nil {
				t.Fatalf("run original %s: %v\n%s", name, err, originalRun)
			}
			files, err := modernDecompileTree(t, dir)
			if err != nil {
				t.Fatalf("%s decompile: %v\n--- source ---\n%s", name, err, source)
			}
			rebuilt := t.TempDir()
			var paths []string
			for fileName, decompiled := range files {
				if strings.Contains(decompiled, "yak-decompiler: undecompilable") {
					t.Fatalf("%s/%s was stubbed:\n%s", name, fileName, decompiled)
				}
				path := filepath.Join(rebuilt, fileName)
				if err := os.WriteFile(path, []byte(decompiled), 0o644); err != nil {
					t.Fatal(err)
				}
				paths = append(paths, path)
			}
			if err := compileTreeAt(t, javac, rebuilt, paths, 21); err != nil {
				t.Fatalf("%s rebuilt tree does not compile: %v", name, err)
			}
			rebuiltRun, err := exec.Command(java, "-Xverify:all", "-cp", rebuilt, name).CombinedOutput()
			if err != nil {
				t.Fatalf("%s rebuilt run failed: %v\n%s", name, err, rebuiltRun)
			}
			if string(rebuiltRun) != string(originalRun) {
				t.Fatalf("%s behavior changed:\nwant %q\ngot  %q", name, originalRun, rebuiltRun)
			}
		}
	}
}
