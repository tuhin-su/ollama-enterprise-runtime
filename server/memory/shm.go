package memory

import (
	"fmt"
	"os"
	"sync"
	"syscall"
	"unsafe"
)

// SharedMemoryBuffer provides direct zero-copy shared memory access across CPU, RAM, and GPU IPC memory mapping.
type SharedMemoryBuffer struct {
	mu       sync.RWMutex
	name     string
	size     int
	file     *os.File
	data     []byte
	isOwner  bool
}

// NewSharedMemoryBuffer creates or opens a POSIX shared memory buffer (/dev/shm) for zero-copy IPC data transfer.
func NewSharedMemoryBuffer(name string, size int) (*SharedMemoryBuffer, error) {
	if size <= 0 {
		size = 64 * 1024 * 1024 // Default 64MB shared buffer pool
	}

	shmPath := "/dev/shm/" + name
	file, err := os.OpenFile(shmPath, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return nil, fmt.Errorf("shared memory: open %s failed: %w", shmPath, err)
	}

	if err := file.Truncate(int64(size)); err != nil {
		file.Close()
		return nil, fmt.Errorf("shared memory: truncate %s failed: %w", shmPath, err)
	}

	data, err := syscall.Mmap(int(file.Fd()), 0, size, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("shared memory: mmap failed: %w", err)
	}

	return &SharedMemoryBuffer{
		name:    name,
		size:    size,
		file:    file,
		data:    data,
		isOwner: true,
	}, nil
}

// ReadBytesZeroCopy reads data from shared memory using direct slice pointers without heap allocation.
func (sm *SharedMemoryBuffer) ReadBytesZeroCopy(offset, length int) ([]byte, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if offset < 0 || offset+length > sm.size {
		return nil, fmt.Errorf("shared memory: out of bounds read offset=%d len=%d cap=%d", offset, length, sm.size)
	}

	// Zero-copy slice view pointing directly into mapped RAM/IPC memory
	return sm.data[offset : offset+length], nil
}

// WriteBytesZeroCopy writes bytes directly into shared memory without copying buffers.
func (sm *SharedMemoryBuffer) WriteBytesZeroCopy(offset int, src []byte) (int, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if offset < 0 || offset+len(src) > sm.size {
		return 0, fmt.Errorf("shared memory: out of bounds write offset=%d len=%d cap=%d", offset, len(src), sm.size)
	}

	copy(sm.data[offset:], src)
	return len(src), nil
}

// StringZeroCopy converts a byte slice into a string pointer without string copy heap allocations.
func StringZeroCopy(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return *(*string)(unsafe.Pointer(&b))
}

// BytesZeroCopy converts a string into a byte slice pointer without byte array copy allocations.
func BytesZeroCopy(s string) []byte {
	if len(s) == 0 {
		return nil
	}
	return *(*[]byte)(unsafe.Pointer(&s))
}

// Close unmaps memory and cleans up shared memory descriptor.
func (sm *SharedMemoryBuffer) Close() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.data != nil {
		_ = syscall.Munmap(sm.data)
		sm.data = nil
	}

	if sm.file != nil {
		sm.file.Close()
		if sm.isOwner {
			_ = os.Remove("/dev/shm/" + sm.name)
		}
		sm.file = nil
	}

	return nil
}
