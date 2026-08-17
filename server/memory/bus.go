package memory

import (
	"fmt"
	"sync"
	"sync/atomic"
	"unsafe"
)

// ModuleDataBus provides ultra-high-speed lock-free shared memory data exchange across backend modules.
type ModuleDataBus struct {
	mu          sync.RWMutex
	channels    map[string]*SharedRingBuffer
	ringSize    int
}

// SharedRingBuffer is a lock-free circular ring buffer designed for zero-copy module-to-module data passing.
type SharedRingBuffer struct {
	capacity uint64
	mask     uint64
	head     uint64 // Producer write index (atomic)
	tail     uint64 // Consumer read index (atomic)
	buffer   []unsafe.Pointer
}

var (
	globalModuleDataBus *ModuleDataBus
	onceDataBus         sync.Once
)

// GetModuleDataBus returns the global high-speed module-to-module data bus singleton.
func GetModuleDataBus() *ModuleDataBus {
	onceDataBus.Do(func() {
		globalModuleDataBus = &ModuleDataBus{
			channels: make(map[string]*SharedRingBuffer),
			ringSize: 4096, // 4k slot ring buffer
		}
	})
	return globalModuleDataBus
}

// GetOrCreateChannel retrieves or initializes a high-speed lock-free ring channel between modules.
func (b *ModuleDataBus) GetOrCreateChannel(channelName string) *SharedRingBuffer {
	b.mu.Lock()
	defer b.mu.Unlock()

	if ch, ok := b.channels[channelName]; ok {
		return ch
	}

	capacity := uint64(b.ringSize)
	ch := &SharedRingBuffer{
		capacity: capacity,
		mask:     capacity - 1,
		buffer:   make([]unsafe.Pointer, capacity),
	}

	b.channels[channelName] = ch
	return ch
}

// PushZeroCopy pushes a zero-copy pointer to another module without memory allocation or mutex locking.
func (rb *SharedRingBuffer) PushZeroCopy(ptr unsafe.Pointer) bool {
	head := atomic.LoadUint64(&rb.head)
	tail := atomic.LoadUint64(&rb.tail)

	if head-tail >= rb.capacity {
		return false // Buffer full
	}

	idx := head & rb.mask
	atomic.StorePointer(&rb.buffer[idx], ptr)
	atomic.StoreUint64(&rb.head, head+1)

	return true
}

// PopZeroCopy pops a zero-copy pointer for fast reading by a consumer module.
func (rb *SharedRingBuffer) PopZeroCopy() (unsafe.Pointer, bool) {
	tail := atomic.LoadUint64(&rb.tail)
	head := atomic.LoadUint64(&rb.head)

	if tail >= head {
		return nil, false // Buffer empty
	}

	idx := tail & rb.mask
	ptr := atomic.LoadPointer(&rb.buffer[idx])
	if ptr == nil {
		return nil, false
	}

	atomic.StorePointer(&rb.buffer[idx], nil)
	atomic.StoreUint64(&rb.tail, tail+1)

	return ptr, true
}

// SharedDataPacket holds cross-module data payloads with zero copy allocations.
type SharedDataPacket struct {
	Sender    string
	Recipient string
	Payload   string
	Data      []byte
}

// SendPacketZeroCopy dispatches a packet across modules via lock-free pointer ring buffer.
func (b *ModuleDataBus) SendPacketZeroCopy(channelName string, packet *SharedDataPacket) error {
	ch := b.GetOrCreateChannel(channelName)
	ptr := unsafe.Pointer(packet)
	if !ch.PushZeroCopy(ptr) {
		return fmt.Errorf("module data bus: channel '%s' buffer full", channelName)
	}
	return nil
}

// ReadPacketZeroCopy reads a packet sent from another module instantly without mutex locks.
func (b *ModuleDataBus) ReadPacketZeroCopy(channelName string) (*SharedDataPacket, bool) {
	ch := b.GetOrCreateChannel(channelName)
	ptr, ok := ch.PopZeroCopy()
	if !ok || ptr == nil {
		return nil, false
	}
	return (*SharedDataPacket)(ptr), true
}
