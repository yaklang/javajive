// Command t28-oracle-probe is a test-only adapter around the public JavaJive API.
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

type observation struct {
	Result javajive.DecompileResult `json:"result"`
	Error  string                   `json:"error,omitempty"`
}

func main() {
	mode := flag.String("mode", "precision", "precision or compatibility")
	budget := flag.Int("max-analysis-updates", 1_000_000, "per-method analysis budget")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: t28-oracle-probe [-mode MODE] classfile")
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
	result, err := javajive.DecompileWithOptions(data, javajive.DecompileOptions{
		Mode: javajive.DecompileMode(*mode), MaxAnalysisUpdates: *budget,
	})
	out := observation{Result: result}
	if err != nil {
		out.Error = err.Error()
	}
	if e := json.NewEncoder(os.Stdout).Encode(out); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(2)
	}
	if err != nil {
		os.Exit(1)
	}
}
