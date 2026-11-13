// Command compressjdk deterministically compresses the reviewed JDK catalog.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/klauspost/compress/zstd"
)

func main() {
	input := flag.String("input", "", "reviewed JDK catalog JSON; output is <input>.zst")
	flag.Parse()
	if *input == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: compressjdk -input jdk_metadata_catalog.json")
		os.Exit(2)
	}
	if err := compress(*input); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func compress(input string) error {
	raw, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	encoder, err := zstd.NewWriter(nil,
		zstd.WithEncoderLevel(zstd.SpeedBestCompression),
		zstd.WithEncoderConcurrency(1),
		zstd.WithWindowSize(8<<20),
		zstd.WithEncoderCRC(true),
	)
	if err != nil {
		return err
	}
	defer encoder.Close()
	compressed := encoder.EncodeAll(raw, nil)
	return os.WriteFile(input+".zst", compressed, 0644)
}
