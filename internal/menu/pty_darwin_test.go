package menu

import (
	"bytes"
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
	var name [128]byte
	err = ioctl(m, syscall.TIOCPTYGRANT, nil)
	if err == nil {
		err = ioctl(m, syscall.TIOCPTYUNLK, nil)
	}
	if err == nil {
		err = ioctl(m, syscall.TIOCPTYGNAME, unsafe.Pointer(&name)) //nolint:gosec // G103: ioctl takes a pointer
	}
	if err != nil {
		_ = m.Close()
		return nil, "", err
	}
	return m, string(name[:bytes.IndexByte(name[:], 0)]), nil
}
