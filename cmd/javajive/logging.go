package main

import (
	"flag"
	"fmt"

	"github.com/yaklang/javajive/internal/log"
)

type logFlagOptions struct {
	quiet    bool
	logLevel string
}

func bindLogFlags(fs *flag.FlagSet) *logFlagOptions {
	opts := &logFlagOptions{}
	fs.BoolVar(&opts.quiet, "q", false, "suppress non-error log and summary output")
	fs.BoolVar(&opts.quiet, "quiet", false, "suppress non-error log and summary output")
	fs.StringVar(&opts.logLevel, "log-level", "", "minimum log level (debug|info|warn|error|quiet)")
	return opts
}

func applyLogFlags(opts *logFlagOptions) (suppressSummary bool, restore func(), err error) {
	if opts == nil {
		return false, func() {}, nil
	}
	if opts.quiet && opts.logLevel != "" {
		return false, func() {}, fmt.Errorf("--quiet and --log-level are mutually exclusive")
	}
	if opts.quiet {
		prev := log.GetLevel()
		log.SetLevel(log.QuietLevel)
		return true, func() { log.SetLevel(prev) }, nil
	}
	if opts.logLevel != "" {
		lvl, parseErr := log.ParseLevel(opts.logLevel)
		if parseErr != nil {
			return false, func() {}, parseErr
		}
		prev := log.GetLevel()
		log.SetLevel(lvl)
		return lvl == log.QuietLevel, func() { log.SetLevel(prev) }, nil
	}
	return false, func() {}, nil
}
