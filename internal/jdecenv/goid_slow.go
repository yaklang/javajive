//go:build !((amd64 || arm64) && !tinygo)

package jdecenv

func gid() uint64 { return gidFromStack() }
