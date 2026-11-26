//go:build windows

package images

import (
	"syscall"
	"unsafe"
)

// diskSpace: o tamanho e o livre do disco onde fica path (go run no Windows).
func diskSpace(path string) (total, free uint64, err error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, err
	}
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW")
	var avail, size, totalFree uint64
	r, _, callErr := proc.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&avail)), uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&totalFree)))
	if r == 0 {
		return 0, 0, callErr
	}
	return size, avail, nil
}
