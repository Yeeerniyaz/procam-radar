//go:build !windows

package main

import "os/exec"

func prepareBackgroundCmd(cmd *exec.Cmd) {
	// No special window flags needed on Unix/Linux
}
