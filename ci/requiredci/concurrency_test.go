package requiredci_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	classparser "github.com/yaklang/javajive/classparser"
)

// concurrentFingerprint runs one decompile with request-local policy and
// returns a canonical JSON fingerprint (or the deterministic error text).
func concurrentFingerprint(data []byte) string {
	result, err := classparser.DecompileWithOptions(data, classparser.DecompileOptions{
		Mode:    classparser.Precision,
		Context: context.Background(),
		Limits: classparser.Limits{
			MaxInputBytes:  1 << 20,
			MaxReadBytes:   2 << 20,
			MaxParseItems:  1 << 16,
			MaxOutputBytes: 2 << 20,
			MaxDiagnostics: 64,
			MaxRequestWork: 4 << 20,
		},
		EnvSnapshot: map[string]string{},
	})
	if err != nil {
		return "error:" + err.Error()
	}
	encoded, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		return "marshal-error:" + marshalErr.Error()
	}
	return string(encoded)
}

// Class decompilation must be deterministic and race-free when callers share
// only package-level caches. The race detector turns any unsynchronized cache
// access into a test failure; the repeated fingerprints catch source drift.
func TestConcurrentDecompileDeterministic(t *testing.T) {
	fixtures := []struct {
		name string
		data []byte
	}{
		{name: "empty", data: []byte{}},
		{name: "magic only", data: []byte{0xca, 0xfe, 0xba, 0xbe}},
		{name: "zeroed class", data: make([]byte, 64)},
	}
	for _, path := range []string{
		filepath.Join("..", "..", "classparser", "testdata", "regression", "FMParser.class"),
		filepath.Join("..", "..", "classparser", "testdata", "regression", "Ser.class"),
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		fixtures = append(fixtures, struct {
			name string
			data []byte
		}{name: filepath.Base(path), data: data})
	}

	want := make([]string, len(fixtures))
	for index, fixture := range fixtures {
		want[index] = concurrentFingerprint(fixture.data)
		if again := concurrentFingerprint(fixture.data); again != want[index] {
			t.Fatalf("%s: sequential decompile is nondeterministic", fixture.name)
		}
	}

	const workers = 3
	start := make(chan struct{})
	failures := make(chan string, workers)
	var wait sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			for index, fixture := range fixtures {
				if got := concurrentFingerprint(fixture.data); got != want[index] {
					failures <- fmt.Sprintf("%s: concurrent result differs", fixture.name)
					return
				}
			}
		}()
	}
	close(start)
	wait.Wait()
	close(failures)
	for failure := range failures {
		t.Error(failure)
	}
}
