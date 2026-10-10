// Copy to <draft-repo>/tools/draft-followup-probe/main.go and build THERE.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/yaklang/javajive"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func read(path string) ([]byte, bool) {
	f, e := os.Open(path)
	if e != nil {
		return nil, false
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, (16<<20)+1))
	return b, e == nil && len(b) <= 16<<20
}
func main() {
	mode := flag.String("mode", "precision", "policy")
	root := flag.String("resolver-root", "", "original class tree, metadata-only resolver")
	shadow := flag.Bool("shadow", false, "collect draft shadow IR")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "need classfile")
		os.Exit(2)
	}
	b, ok := read(flag.Arg(0))
	if !ok {
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	opt := javajive.DecompileOptions{Mode: javajive.DecompileMode(*mode), MaxAnalysisUpdates: 1000000, Context: ctx, EnvSnapshot: map[string]string{}, EnableShadowIR: *shadow}
	if *root != "" {
		r, e := filepath.EvalSymlinks(*root)
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(2)
		}
		r, _ = filepath.Abs(r)
		opt.Resolve = func(n string) ([]byte, bool) {
			if n == "" || strings.ContainsAny(n, ".\\:\x00") || strings.HasPrefix(n, "/") || strings.Contains(n, "//") {
				return nil, false
			}
			p, e := filepath.EvalSymlinks(filepath.Join(r, filepath.FromSlash(n)+".class"))
			if e != nil {
				return nil, false
			}
			rel, e := filepath.Rel(r, p)
			if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
				return nil, false
			}
			return read(p)
		}
	}
	result, err := javajive.DecompileWithOptions(b, opt)
	errorText := ""
	if err != nil {
		errorText = err.Error()
	}
	if e := json.NewEncoder(os.Stdout).Encode(map[string]any{"result": result, "error": errorText}); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(2)
	}
	if err != nil {
		os.Exit(1)
	}
}
