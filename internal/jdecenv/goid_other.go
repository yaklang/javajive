//go:build !amd64

package jdecenv

import "unsafe"

func currentG() unsafe.Pointer { return nil }
