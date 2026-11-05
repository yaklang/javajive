//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package main

import "os/exec"

func configureProcessTree(cmd *exec.Cmd) {}
