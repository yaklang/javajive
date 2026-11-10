// Command javajive is a portable, pure-Go command line tool for working with
// Java and Android artifacts: decompiling .class files and Java archives,
// translating DEX/APK/APKS/XAPK/APKM/AAB inputs, inspecting class structure,
// and parsing/marshaling the Java serialization wire format.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	jj "github.com/yaklang/javajive"
)

const version = jj.Version

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
	case "dex2jar", "d2j":
		err = cmdDexToJar(args)
	case "jimage2jar", "i2j":
		err = cmdJImageToJar(args)
	case "inspect", "analyze":
		err = cmdInspect(args)
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
  decompile   Decompile .class files, Java archives, JImage runtime images, or DEX/APK/APKS/XAPK/APKM/AAB input
  dex2jar     Convert DEX/APK/APKS/XAPK/APKM/AAB input to a deterministic JAR
  jimage2jar  Convert a JDK runtime image (lib/modules, .jimage) to a JAR view
  inspect     Read-only inventory and best-effort analysis of Java/Android artifacts

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
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: javajive decompile [-o out] [-analysis-budget N] [--structured] <file|dir>")
		fmt.Fprintln(fs.Output(), "Decompile .class, Java archives (.jar/.war/.ear/.aar/.par/.jmod/.zip), JDK runtime images (.jimage / lib/modules), DEX/APK/APKS/XAPK/APKM/AAB, or a directory of .class files.")
		fmt.Fprintln(fs.Output(), "Android containers and DEX default to <input>.src; --structured applies to Java archives and JImage.")
		fs.PrintDefaults()
	}
	out := fs.String("o", "", "output file (for a .class) or directory (for archives/DEX/APK/dirs); default: stdout / <input>.src")
	budget := fs.Int("analysis-budget", 0, "per-method analysis updates for very large generated methods (0 = default; >1000000 scales graph caps)")
	structured := fs.Bool("structured", false, "for archives: emit sources/, resources/, and preserved nested archives/ views")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *budget < 0 {
		return fmt.Errorf("-analysis-budget must be non-negative")
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
		return decompileDir(input, *out, *budget)
	}

	kind, err := jj.ClassifyInputFile(input)
	if err != nil {
		return err
	}

	if jj.IsAndroidContainer(kind) {
		if *structured {
			return fmt.Errorf("--structured applies to Java archives, not %s input", kind)
		}
		dst := *out
		if dst == "" {
			dst = input + ".src"
		}
		if err := jj.DecompileDEXFileWithAnalysisBudget(input, dst, jj.DexOptions{}, *budget); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "decompiled %s input %q -> %q\n", kind, input, dst)
		return nil
	}

	if jj.IsJavaArchiveContainer(kind) {
		dst := *out
		if dst == "" {
			dst = input + ".src"
		}
		if *structured {
			report, err := jj.DecompileArchiveWithOptions(input, dst, jj.ArchiveOptions{
				Structured:         true,
				MaxAnalysisUpdates: *budget,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "decompiled %s archive %q -> %q (%d entries, %d nested archives)\n",
				kind, input, dst, len(report.Entries), len(report.Archives))
			return nil
		}
		if err := jj.DecompileArchiveWithAnalysisBudget(input, dst, *budget); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "decompiled %s archive %q -> %q\n", kind, input, dst)
		return nil
	}

	data, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	src, err := decompileClassBytes(data, *budget)
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

// decompileClassBytes uses the explicit-budget path only when the caller asked
// for one, preserving the default single-class behavior otherwise.
func decompileClassBytes(data []byte, budget int) (string, error) {
	if budget <= 0 {
		return jj.Decompile(data)
	}
	result, err := jj.DecompileWithOptions(data, jj.DecompileOptions{
		Mode:               jj.Compatibility,
		MaxAnalysisUpdates: budget,
	})
	if err != nil {
		return "", err
	}
	if result.Source == "" {
		return "", fmt.Errorf("decompile produced no source (status=%s)", result.Status)
	}
	return result.Source, nil
}

func decompileDir(dir, out string, budget int) error {
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
		src, decErr := decompileClassBytes(data, budget)
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
// dex2jar
// ---------------------------------------------------------------------------

func cmdDexToJar(args []string) error {
	fs := newFlagSet("dex2jar")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: javajive dex2jar -o output.jar input.dex|input.apk|input.apks|input.xapk|input.apkm|input.aab")
		fmt.Fprintln(fs.Output(), "Convert the supported DEX subset in a DEX/APK/APKS/XAPK/APKM/AAB input to a deterministic JAR.")
		fs.PrintDefaults()
	}
	out := fs.String("o", "", "output JAR path (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 || *out == "" {
		return fmt.Errorf("dex2jar requires -o <output.jar> and one input path\nusage: javajive dex2jar -o output.jar input.dex|input.apk|input.apks|input.xapk|input.apkm|input.aab")
	}
	kind, _ := jj.ClassifyInputFile(fs.Arg(0))
	if kind == "" {
		kind = jj.ContainerUnknown
	}
	report, err := jj.DexFileToJar(fs.Arg(0), *out, jj.DexOptions{})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "converted %s %q -> %q (%d DEX files, %d classes, %d/%d methods translated, %d warnings)\n",
		kind, fs.Arg(0), *out, report.DEXFiles, report.Classes, report.TranslatedMethods, report.Methods, report.Warnings)
	return nil
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
	obj, err := jj.ParseClass(data)
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

	var objs []jj.JavaSerializable
	if *asHex {
		objs, err = jj.ParseSerializedHex(string(raw))
	} else {
		objs, err = jj.ParseSerialized(raw)
	}
	if err != nil {
		return err
	}
	jsonBytes, err := jj.SerializedToJSON(objs...)
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
	objs, err := jj.SerializedFromJSON(raw)
	if err != nil {
		return err
	}

	// When writing to a terminal/stdout without -o, default to hex for safety.
	emitHex := *asHex || (*out == "")
	if emitHex {
		return writeOutput(*out, []byte(jj.MarshalSerializedHex(objs...)+"\n"), false)
	}
	return writeOutput(*out, jj.MarshalSerialized(objs...), false)
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
