package menu

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// openPTY opens a pseudo terminal and returns its master and the path of its
// slave.
func openPTY() (*os.File, string, error) {
	m, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, "", err
	}
	var unlock int32
	var n uint32
	if err := ioctl(m, syscall.TIOCSPTLCK, unsafe.Pointer(&unlock)); err != nil { //nolint:gosec // G103: ioctl takes a pointer
		_ = m.Close()
		return nil, "", err
	}
	if err := ioctl(m, syscall.TIOCGPTN, unsafe.Pointer(&n)); err != nil { //nolint:gosec // G103: ioctl takes a pointer
		_ = m.Close()
		return nil, "", err
	}
	return m, fmt.Sprintf("/dev/pts/%d", n), nil
}
