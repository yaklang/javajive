package requiredci_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

var processDeterminismFixtures = []string{
	"FMParser.class",
	"nondeterministic_loop_structuring.class",
}

const processDeterminismMarker = "JAVAJIVE_DETERMINISM"

func processDeterminismHash(name string) (string, error) {
	data, err := os.ReadFile(filesystemFixturePath(name))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(concurrentFingerprint(data)))
	return fmt.Sprintf("%s %x", name, sum), nil
}

func filesystemFixturePath(name string) string {
	return "../../classparser/testdata/regression/" + name
}

// The child is invoked by TestDecompileDeterministicAcrossProcesses with a
// private environment. A normal full-suite run returns without doing work.
func TestDecompileDeterminismChild(t *testing.T) {
	if os.Getenv("JAVAJIVE_DETERMINISM_CHILD") != "1" {
		return
	}
	for _, name := range processDeterminismFixtures {
		line, err := processDeterminismHash(name)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Printf("%s %s\n", processDeterminismMarker, line)
	}
}

// Map iteration order is seeded per process. Re-running the same decompile in
// child processes proves source, diagnostics, records and hashes do not depend
// on that seed, which same-process repetition cannot show.
func TestDecompileDeterministicAcrossProcesses(t *testing.T) {
	want := make(map[string]string, len(processDeterminismFixtures))
	for _, name := range processDeterminismFixtures {
		line, err := processDeterminismHash(name)
		if err != nil {
			t.Fatal(err)
		}
		want[name] = line
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for run := 0; run < 2; run++ {
		command := exec.Command(executable, "-test.run=^TestDecompileDeterminismChild$", "-test.count=1", "-test.timeout=2m")
		environment := make([]string, 0, len(os.Environ())+1)
		for _, entry := range os.Environ() {
			if strings.HasPrefix(entry, "JAVAJIVE_DETERMINISM_CHILD=") {
				continue
			}
			environment = append(environment, entry)
		}
		command.Env = append(environment, "JAVAJIVE_DETERMINISM_CHILD=1")
		output, runErr := command.CombinedOutput()
		if runErr != nil {
			t.Fatalf("child run %d: %v\n%s", run, runErr, output)
		}
		seen := map[string]bool{}
		for _, line := range strings.Split(string(output), "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, processDeterminismMarker+" ") {
				continue
			}
			got := strings.TrimPrefix(line, processDeterminismMarker+" ")
			name := strings.SplitN(got, " ", 2)[0]
			if expected, ok := want[name]; !ok || got != expected {
				t.Fatalf("child run %d %s: got %q want %q", run, name, got, expected)
			}
			seen[name] = true
		}
		if len(seen) != len(want) {
			t.Fatalf("child run %d: saw %d fixtures, want %d\n%s", run, len(seen), len(want), output)
		}
	}
}
