//go:build linux || darwin

package menu

import (
	"os"
	"syscall"
	"unsafe"
)

// ioctl calls ioctl(2) on f; arg may be nil.
func ioctl(f *os.File, req uintptr, arg unsafe.Pointer) error {
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), req, uintptr(arg)); errno != 0 {
		return errno
	}
	return nil
}
