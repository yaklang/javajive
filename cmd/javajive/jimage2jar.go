package main

import (
	"fmt"
	"os"

	jj "github.com/yaklang/javajive"
)

func cmdJImageToJar(args []string) error {
	fs := newFlagSet("jimage2jar")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: javajive jimage2jar -o output.jar <lib/modules|input.jimage>")
		fmt.Fprintln(fs.Output(), "Convert a JDK runtime image into a package-layout JAR view.")
		fs.PrintDefaults()
	}
	out := fs.String("o", "", "output JAR path (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 || *out == "" {
		return fmt.Errorf("jimage2jar requires -o <output.jar> and one input path\nusage: javajive jimage2jar -o output.jar <lib/modules|input.jimage>")
	}
	if err := jj.JImageToJar(fs.Arg(0), *out); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "converted JImage %q -> %q\n", fs.Arg(0), *out)
	return nil
}
