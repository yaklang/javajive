//go:build arm64

package jdecenv

import "unsafe"

func currentG() unsafe.Pointer
