//go:build !windows

package main

import (
	"net"
	"os/exec"
	"syscall"
)

func prepareBackgroundCmd(cmd *exec.Cmd) {
	// No special window flags needed on Unix/Linux
}

func bindUDPConnToDevice(conn *net.UDPConn, ifaceName string) {
	if conn == nil || ifaceName == "" {
		return
	}
	rawConn, err := conn.SyscallConn()
	if err != nil {
		return
	}
	_ = rawConn.Control(func(fd uintptr) {
		_ = syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, ifaceName)
	})
}
