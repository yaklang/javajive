package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

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
		fmt.Fprintln(os.Stderr, "usage: javajive inspect INPUT --format json")
		fmt.Fprintln(os.Stderr, "Read-only inventory and best-effort analysis of DEX, APK, JAR, and WAR artifacts.")
		fs.PrintDefaults()
	}
	format := fs.String("format", "json", "report format (json)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("inspect requires exactly one input path\nusage: javajive inspect INPUT --format json")
	}
	if *format != "json" {
		return fmt.Errorf("unsupported inspect format %q (want json)", *format)
	}

	report, err := artifactinspect.AnalyzeFile(fs.Arg(0))
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(report)
}

// flag.FlagSet stops parsing at the first positional argument. Normalize the
// documented INPUT --format json spelling while retaining ordinary flag-first
// syntax and letting FlagSet produce all validation errors.
func normalizeInspectArgs(args []string) []string {
	if len(args) < 2 || len(args[0]) == 0 || args[0][0] == '-' {
		return args
	}
	if args[1] != "--format" && args[1] != "-format" && len(args) == 2 {
		return args
	}
	if args[1] == "--format" || args[1] == "-format" {
		if len(args) < 3 {
			return args
		}
		out := []string{args[1], args[2], args[0]}
		return append(out, args[3:]...)
	}
	if len(args) == 2 && (strings.HasPrefix(args[1], "--format=") || strings.HasPrefix(args[1], "-format=")) {
		return []string{args[1], args[0]}
	}
	return args
}
