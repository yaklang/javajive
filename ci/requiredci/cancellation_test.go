package requiredci_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	classparser "github.com/yaklang/javajive/classparser"
)

func cancellationOptions(ctx context.Context) classparser.DecompileOptions {
	return classparser.DecompileOptions{
		Mode:    classparser.Precision,
		Context: ctx,
		Limits: classparser.Limits{
			MaxInputBytes:  1 << 20,
			MaxReadBytes:   2 << 20,
			MaxParseItems:  1 << 16,
			MaxOutputBytes: 2 << 20,
			MaxDiagnostics: 64,
			MaxRequestWork: 4 << 20,
		},
		EnvSnapshot: map[string]string{},
	}
}

func readCancellationFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "classparser", "testdata", "regression", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// A canceled request must abort without publishing partial source, and the
// cancellation class must survive the work-budget wrapper.
func TestDecompileCancellationFailsClosed(t *testing.T) {
	for _, name := range []string{"FMParser.class", "nondeterministic_loop_structuring.class"} {
		data := readCancellationFixture(t, name)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		result, err := classparser.DecompileWithOptions(data, cancellationOptions(ctx))
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("%s pre-canceled: err=%v status=%s", name, err, result.Status)
		}
		if result.Source != "" {
			t.Fatalf("%s pre-canceled: published %d source bytes", name, len(result.Source))
		}

		ctx, cancel = context.WithCancel(context.Background())
		go func() {
			time.Sleep(time.Millisecond)
			cancel()
		}()
		result, err = classparser.DecompileWithOptions(data, cancellationOptions(ctx))
		cancel()
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("%s mid-flight: err=%v status=%s", name, err, result.Status)
		}
		if result.Source != "" {
			t.Fatalf("%s mid-flight: published %d source bytes", name, len(result.Source))
		}
	}
}
