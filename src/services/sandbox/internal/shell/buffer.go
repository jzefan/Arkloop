package shell

const (
	RingBufferBytes = 1 << 20
	ReadChunkBytes  = 64 * 1024
)

type RingBuffer struct {
	maxSize     int
	startCursor uint64
	endCursor   uint64
	buf         []byte
}

func NewRingBuffer(maxSize int) *RingBuffer {
	if maxSize <= 0 {
		maxSize = RingBufferBytes
	}
	return &RingBuffer{
		maxSize: maxSize,
		buf:     make([]byte, 0, minInt(maxSize, 4096)),
	}
}

func (b *RingBuffer) Append(data []byte) {
	if len(data) == 0 {
		return
	}
	if len(data) >= b.maxSize {
		trimmed := data[len(data)-b.maxSize:]
		b.startCursor = b.endCursor + uint64(len(data)-b.maxSize)
		b.endCursor += uint64(len(data))
		b.buf = append(b.buf[:0], trimmed...)
		return
	}

	b.buf = append(b.buf, data...)
	b.endCursor += uint64(len(data))

	over := len(b.buf) - b.maxSize
	if over <= 0 {
		return
	}
	b.buf = append([]byte(nil), b.buf[over:]...)
	b.startCursor += uint64(over)
}

func (b *RingBuffer) ReadFrom(cursor uint64, limit int) ([]byte, uint64, bool, bool) {
	if limit <= 0 {
		limit = ReadChunkBytes
	}
	if cursor > b.endCursor {
		return nil, 0, false, false
	}

	truncated := false
	if cursor < b.startCursor {
		cursor = b.startCursor
		truncated = true
	}
	if cursor == b.endCursor {
		return nil, b.endCursor, truncated, true
	}

	offset := int(cursor - b.startCursor)
	available := len(b.buf) - offset
	if available < 0 {
		available = 0
	}
	if available > limit {
		available = limit
	}
	if available == 0 {
		return nil, cursor, truncated, true
	}

	chunk := append([]byte(nil), b.buf[offset:offset+available]...)
	return chunk, cursor + uint64(available), truncated, true
}

func (b *RingBuffer) EndCursor() uint64 {
	return b.endCursor
}

func (b *RingBuffer) StartCursor() uint64 {
	return b.startCursor
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
