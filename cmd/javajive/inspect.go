package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	artifactinspect "github.com/yaklang/javajive/inspect"
)

func cmdInspect(args []string) error {
	return cmdInspectTo(args, os.Stdout)
}

func cmdInspectTo(args []string, stdout io.Writer) error {
	args = normalizeInspectArgs(args)
	fs := newFlagSet("inspect")
	fs.SetOutput(io.Discard)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: javajive inspect INPUT --format json [-q|--quiet] [--log-level level] [--disassembly=false]")
		fmt.Fprintln(os.Stderr, "Read-only inventory and best-effort analysis of Java archives, JDK runtime images, and Android containers.")
		fs.PrintDefaults()
	}
	format := fs.String("format", "json", "report format (json)")
	disassembly := fs.Bool("disassembly", true, "include per-instruction DEX disassembly; disable for very large applications")
	logOpts := bindLogFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	_, restoreLog, err := applyLogFlags(logOpts)
	if err != nil {
		return err
	}
	defer restoreLog()
	if fs.NArg() != 1 {
		return fmt.Errorf("inspect requires exactly one input path\nusage: javajive inspect INPUT --format json")
	}
	if *format != "json" {
		return fmt.Errorf("unsupported inspect format %q (want json)", *format)
	}

	report, err := artifactinspect.AnalyzeFileWithOptions(fs.Arg(0), artifactinspect.Options{Disassembly: *disassembly})
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(report)
}

// flag.FlagSet stops parsing at the first positional argument. Normalize the
// documented INPUT --format json [flags...] spelling while retaining ordinary
// flag-first syntax and letting FlagSet produce all validation errors.
func normalizeInspectArgs(args []string) []string {
	if len(args) < 2 || len(args[0]) == 0 || args[0][0] == '-' || len(args[1]) == 0 || args[1][0] != '-' {
		return args
	}
	flags := make([]string, 0, len(args))
	pos := []string{args[0]}
	for i := 1; i < len(args); i++ {
		tok := args[i]
		if tok == "--" {
			pos = append(pos, args[i:]...)
			break
		}
		if len(tok) == 0 || tok[0] != '-' {
			pos = append(pos, tok)
			continue
		}
		flags = append(flags, tok)
		if (tok == "--format" || tok == "-format" || tok == "--log-level" || tok == "-log-level") && i+1 < len(args) {
			flags = append(flags, args[i+1])
			i++
		}
	}
	return append(flags, pos...)
}
