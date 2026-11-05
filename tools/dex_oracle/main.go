// Command dex_oracle fetches pinned dex2jar distributions and probes DEX inputs.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "fetch":
		err = runFetch(os.Args[2:])
	case "probe":
		err = runProbe(os.Args[2:])
	case "help", "-h", "--help":
		usage()
		return
	default:
		usage()
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "dex_oracle:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  dex_oracle fetch [--cache DIR] [--manifest FILE] [--versions LIST]
  dex_oracle probe [--cache DIR] [--manifest FILE] [--versions LIST] [--output FILE] INPUT...

probe uses tools/sandbox_worker by default. --allow-untrusted-host-execution is an explicit unsafe bypass.

LIST is a comma-separated set of manifest ids or versions; default is all.`)
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}
