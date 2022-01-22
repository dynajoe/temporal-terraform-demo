//go:build !linux
// +build !linux

package tfexec

import (
	"syscall"
)

func osSpecificSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		// set process group ID
		Setpgid: true,
	}
}
