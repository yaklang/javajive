// Command t01-api-probe is a test-only adapter around javajive.DecompileWithOptions.
// It is not a production CLI and does not change decompiler algorithms.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/yaklang/javajive"
)

func main() {
	modeFlag := flag.String("mode", "precision", "precision or compatibility")
	budget := flag.Int("max-analysis-updates", 1_000_000, "MaxAnalysisUpdates")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: t01-api-probe -mode precision|compatibility <classfile>")
		os.Exit(2)
	}

	var mode javajive.DecompileMode
	switch *modeFlag {
	case string(javajive.Precision):
		mode = javajive.Precision
	case string(javajive.Compatibility):
		mode = javajive.Compatibility
	default:
		fmt.Fprintf(os.Stderr, "unknown mode %q\n", *modeFlag)
		os.Exit(2)
	}

	f, err := os.Open(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	const maxInput = 16 << 20
	data, err := io.ReadAll(io.LimitReader(f, maxInput+1))
	_ = f.Close()
	if err != nil || len(data) > maxInput {
		fmt.Fprintf(os.Stderr, "cannot read bounded class input: %v\n", err)
		os.Exit(2)
	}

	result, decErr := javajive.DecompileWithOptions(data, javajive.DecompileOptions{
		Mode:               mode,
		MaxAnalysisUpdates: *budget,
	})

	var errField any
	if decErr != nil {
		errField = decErr.Error()
	}

	payload := map[string]any{
		"source":        result.Source,
		"err":           errField,
		"status":        result.Status,
		"stub_methods":  result.StubMethods,
		"mode":          result.Mode,
		"input_hash":    result.InputHash,
		"diagnostics":   result.Diagnostics,
		"rules_applied": result.RulesApplied,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	if e := enc.Encode(payload); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(2)
	}
	if decErr != nil {
		os.Exit(1)
	}
}
