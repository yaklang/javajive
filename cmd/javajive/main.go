// Command javajive is a portable, pure-Go command line tool for working with Java
// artifacts: decompiling .class/.jar/.war files, inspecting class structure, and
// parsing/marshaling the Java serialization wire format.
package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	classparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/classparser/jarwar"
	yserx "github.com/yaklang/javajive/serialization"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "decompile", "dec":
		err = cmdDecompile(args)
	case "classinfo", "info":
		err = cmdClassInfo(args)
	case "serial", "ser":
		err = cmdSerial(args)
	case "version", "-v", "--version":
		fmt.Println("javajive", version)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `javajive - portable pure-Go Java tooling

Usage:
  javajive <command> [arguments]

Commands:
  decompile   Decompile .class/.jar/.war/.zip or a directory into Java source
  classinfo   Print the structure of a .class file (version, fields, methods)
  serial      Java serialization tools (subcommands: tojson, fromjson)
  version     Print the javajive version
  help        Show this help

Run "javajive <command> -h" for command-specific options.
`)
}

// ---------------------------------------------------------------------------
// decompile
// ---------------------------------------------------------------------------

func cmdDecompile(args []string) error {
	fs := newFlagSet("decompile")
	out := fs.String("o", "", "output file (for a .class) or directory (for archives/dirs); default: stdout / <input>.src")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("decompile requires exactly one input path\nusage: javajive decompile [-o out] <file|dir>")
	}
	input := fs.Arg(0)

	info, err := os.Stat(input)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return decompileDir(input, *out)
	}

	switch strings.ToLower(filepath.Ext(input)) {
	case ".jar", ".war", ".zip", ".jmod", ".ear":
		dst := *out
		if dst == "" {
			dst = input + ".src"
		}
		if err := jarwar.AutoDecompile(input, dst); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "decompiled archive %q -> %q\n", input, dst)
		return nil
	default:
		data, err := os.ReadFile(input)
		if err != nil {
			return err
		}
		src, err := classparser.Decompile(data)
		if err != nil {
			return err
		}
		if *out == "" {
			fmt.Print(src)
			if !strings.HasSuffix(src, "\n") {
				fmt.Println()
			}
			return nil
		}
		return os.WriteFile(*out, []byte(src), 0o644)
	}
}

func decompileDir(dir, out string) error {
	if out == "" {
		return fmt.Errorf("decompiling a directory requires -o <output directory>")
	}
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.EqualFold(filepath.Ext(path), ".class") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		src, decErr := classparser.Decompile(data)
		if decErr != nil {
			fmt.Fprintf(os.Stderr, "skip %q: %v\n", path, decErr)
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		dst := filepath.Join(out, strings.TrimSuffix(rel, filepath.Ext(rel))+".java")
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dst, []byte(src), 0o644)
	})
}

// ---------------------------------------------------------------------------
// classinfo
// ---------------------------------------------------------------------------

func cmdClassInfo(args []string) error {
	fs := newFlagSet("classinfo")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("classinfo requires exactly one .class file\nusage: javajive classinfo <file.class>")
	}
	data, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	obj, err := classparser.Parse(data)
	if err != nil {
		return err
	}

	utf8 := func(index uint16) string {
		if obj.ConstantPoolManager == nil {
			return ""
		}
		if info := obj.ConstantPoolManager.GetUtf8(int(index)); info != nil {
			return info.Value
		}
		return ""
	}

	fmt.Printf("class:      %s\n", obj.GetClassName())
	fmt.Printf("super:      %s\n", obj.GetSupperClassName())
	fmt.Printf("version:    %d.%d\n", obj.MajorVersion, obj.MinorVersion)
	fmt.Printf("access:     %s\n", strings.Join(obj.AccessFlagsVerbose, " "))
	if ifaces := obj.GetInterfacesName(); len(ifaces) > 0 {
		fmt.Printf("interfaces: %s\n", strings.Join(ifaces, ", "))
	}
	fmt.Printf("constants:  %d\n", len(obj.ConstantPool))

	fmt.Printf("\nfields (%d):\n", len(obj.Fields))
	for _, f := range obj.Fields {
		fmt.Printf("  %s %s %s\n", strings.Join(f.AccessFlagsVerbose, " "), utf8(f.DescriptorIndex), utf8(f.NameIndex))
	}
	fmt.Printf("\nmethods (%d):\n", len(obj.Methods))
	for _, m := range obj.Methods {
		fmt.Printf("  %s %s%s\n", strings.Join(m.AccessFlagsVerbose, " "), utf8(m.NameIndex), utf8(m.DescriptorIndex))
	}
	return nil
}

// ---------------------------------------------------------------------------
// serial
// ---------------------------------------------------------------------------

func cmdSerial(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("serial requires a subcommand: tojson | fromjson")
	}
	sub := args[0]
	rest := args[1:]
	switch sub {
	case "tojson", "parse":
		return cmdSerialToJSON(rest)
	case "fromjson", "marshal":
		return cmdSerialFromJSON(rest)
	default:
		return fmt.Errorf("unknown serial subcommand %q (want tojson | fromjson)", sub)
	}
}

func cmdSerialToJSON(args []string) error {
	fs := newFlagSet("serial tojson")
	asHex := fs.Bool("hex", false, "treat the input as a hex string")
	out := fs.String("o", "", "output file (default: stdout)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	raw, err := readInput(fs)
	if err != nil {
		return err
	}

	var objs []yserx.JavaSerializable
	if *asHex {
		objs, err = yserx.ParseHexJavaSerialized(strings.TrimSpace(string(raw)))
	} else {
		objs, err = yserx.ParseJavaSerialized(raw)
	}
	if err != nil {
		return err
	}
	jsonBytes, err := yserx.ToJson(objs)
	if err != nil {
		return err
	}
	return writeOutput(*out, jsonBytes, true)
}

func cmdSerialFromJSON(args []string) error {
	fs := newFlagSet("serial fromjson")
	asHex := fs.Bool("hex", false, "emit the marshaled object stream as a hex string instead of raw bytes")
	out := fs.String("o", "", "output file (default: stdout)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	raw, err := readInput(fs)
	if err != nil {
		return err
	}
	objs, err := yserx.FromJson(raw)
	if err != nil {
		return err
	}
	marshaled := yserx.MarshalJavaObjects(objs...)

	// When writing to a terminal/stdout without -o, default to hex for safety.
	emitHex := *asHex || (*out == "")
	if emitHex {
		return writeOutput(*out, []byte(hex.EncodeToString(marshaled)+"\n"), false)
	}
	return writeOutput(*out, marshaled, false)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func newFlagSet(name string) *flag.FlagSet {
	return flag.NewFlagSet(name, flag.ContinueOnError)
}

// readInput reads the single positional argument as a file path, or stdin if it
// is "-" or omitted.
func readInput(fs *flag.FlagSet) ([]byte, error) {
	if fs.NArg() == 0 || fs.Arg(0) == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(fs.Arg(0))
}

func writeOutput(path string, data []byte, ensureTrailingNewline bool) error {
	if path == "" {
		os.Stdout.Write(data)
		if ensureTrailingNewline && (len(data) == 0 || data[len(data)-1] != '\n') {
			fmt.Println()
		}
		return nil
	}
	return os.WriteFile(path, data, 0o644)
}
