package reader

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

// Open takes a string path to a MaxMind DB file and returns a Reader
// structure or an error. The database file is opened using a memory map
// on supported platforms. On platforms without memory map support, such
// as WebAssembly or Google App Engine, or if the memory map attempt fails
// due to lack of support from the filesystem, the database is loaded into memory.
// Use the Close method on the Reader object to return the resources to the system.

// MMAP UNIX
type mmapENODEVError struct{}

func (mmapENODEVError) Error() string {
	return "mmap: файловая система не поддерживает отображение в память"
}

func (mmapENODEVError) Is(target error) bool {
	return target == errors.ErrUnsupported
}

func mmap(fd, length int) ([]byte, error) {
	ptr, _, errno := syscall.Syscall6(syscall.SYS_MMAP, 0, uintptr(length), syscall.PROT_READ, syscall.MAP_SHARED, uintptr(fd), 0)
	if errno != 0 {
		if errno == syscall.ENODEV {
			return nil, mmapENODEVError{}
		}
		return nil, os.NewSyscallError("mmap", errno)
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(ptr)), length), nil
}

func munmap(b []byte) error {
	if _, _, errno := syscall.Syscall(syscall.SYS_MUNMAP, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), 0); errno != 0 {
		return os.NewSyscallError("munmap", errno)
	}
	return nil
}

/*type mmapENODEVError struct{}

func (mmapENODEVError) Error() string {
	return "mmap: the underlying filesystem of the specified file does not support memory mapping"
}

func (mmapENODEVError) Is(target error) bool {
	return target == errors.ErrUnsupported
}

func mmap(fd, length int) (data []byte, err error) {
	data, err = unix.Mmap(fd, 0, length, unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		if err == unix.ENODEV {
			return nil, mmapENODEVError{}
		}
		return nil, os.NewSyscallError("mmap", err)
	}
	return data, nil
}

func munmap(b []byte) (err error) {
	if err = unix.Munmap(b); err != nil {
		return os.NewSyscallError("munmap", err)
	}
	return nil
}*/
