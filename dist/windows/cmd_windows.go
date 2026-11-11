//go:build windows

package main

import (
	"net"
	"os/exec"
	"syscall"
)

func prepareBackgroundCmd(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}

func bindUDPConnToDevice(conn *net.UDPConn, ifaceName string) {
	// On Windows, binding UDP socket to the interface's local IPv4 address routes traffic through that NIC.
}
