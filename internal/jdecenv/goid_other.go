//go:build !amd64 && !arm64

package jdecenv

import "unsafe"

func currentG() unsafe.Pointer { return nil }
