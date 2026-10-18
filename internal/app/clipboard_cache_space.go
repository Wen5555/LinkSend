package app

import (
	"errors"
	"math"
	"sync"
	"syscall"

	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/transfer"
)

// Owned by Service's existing profile owner. Reservations remain live through
// receive, verification and native publication, including cancellation/failure.
type clipboardCacheSpace struct {
	mu           sync.Mutex
	reservations map[string]uint64
	available    func(string) (uint64, error)
}

func clipboardCacheSpaceRequired(m transfer.Manifest) (uint64, error) {
	if err := m.Validate(); err != nil {
		return 0, err
	}
	required := uint64(16<<20) + uint64(len(m.Files))*4096
	for _, entry := range m.Files {
		if entry.Size < 0 || uint64(entry.Size) > math.MaxUint64-required {
			return 0, errors.New("RESOURCE_LIMIT")
		}
		required += uint64(entry.Size)
	}
	return required, nil
}

func (c *clipboardCacheSpace) reserve(directory string, m transfer.Manifest) (func(), error) {
	required, err := clipboardCacheSpaceRequired(m)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.reservations[m.TransferID]; exists {
		return nil, errors.New("CLIPBOARD_CACHE_RESERVATION_EXISTS")
	}
	probe := c.available
	if probe == nil {
		probe = availableDiskBytes
	}
	available, err := probe(directory)
	if err != nil {
		return nil, protocol.Wrap(protocol.ReceiveDirectoryUnavailable, "无法检查复制文件所在磁盘。", err)
	}
	guard := max(uint64(64<<20), min(uint64(1<<30), available/100))
	if available < guard || required > available-guard {
		return nil, protocol.Wrap(protocol.DiskFull, "空间不足，暂时无法复制文件。", syscall.ENOSPC)
	}
	remaining := available - guard - required
	for _, reserved := range c.reservations {
		if reserved > remaining {
			return nil, protocol.Wrap(protocol.DiskFull, "空间不足，暂时无法复制文件。", syscall.ENOSPC)
		}
		remaining -= reserved
	}
	if c.reservations == nil {
		c.reservations = make(map[string]uint64)
	}
	c.reservations[m.TransferID] = required
	var once sync.Once
	return func() {
		once.Do(func() {
			c.mu.Lock()
			delete(c.reservations, m.TransferID)
			c.mu.Unlock()
		})
	}, nil
}
