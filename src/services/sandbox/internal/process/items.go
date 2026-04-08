package process

const (
	defaultItemBufferBytes = 1 << 20
	defaultResponseBytes   = 32 * 1024
	maxItemChunkBytes      = 4 * 1024
)

type ItemBuffer struct {
	maxBytes int
	headSeq  uint64
	nextSeq  uint64
	bytes    int
	items    []OutputItem
}

func NewItemBuffer(maxBytes int) *ItemBuffer {
	if maxBytes <= 0 {
		maxBytes = defaultItemBufferBytes
	}
	return &ItemBuffer{maxBytes: maxBytes}
}

func (b *ItemBuffer) Append(stream, text string) {
	if text == "" {
		return
	}
	for len(text) > 0 {
		chunk := text
		if len(chunk) > maxItemChunkBytes {
			chunk = chunk[:maxItemChunkBytes]
		}
		b.appendChunk(stream, chunk)
		text = text[len(chunk):]
	}
}

func (b *ItemBuffer) appendChunk(stream, text string) {
	item := OutputItem{
		Seq:    b.nextSeq,
		Stream: stream,
		Text:   text,
	}
	b.nextSeq++
	b.items = append(b.items, item)
	b.bytes += len(item.Text)
	for b.bytes > b.maxBytes && len(b.items) > 0 {
		b.bytes -= len(b.items[0].Text)
		b.items = b.items[1:]
		b.headSeq++
	}
	if len(b.items) == 0 {
		b.headSeq = b.nextSeq
	}
}

func (b *ItemBuffer) HeadSeq() uint64 {
	return b.headSeq
}

func (b *ItemBuffer) NextSeq() uint64 {
	return b.nextSeq
}

func (b *ItemBuffer) ReadFrom(cursor uint64, limit int) (items []OutputItem, next uint64, hasMore bool, truncated bool, ok bool) {
	if limit <= 0 {
		limit = defaultResponseBytes
	}
	if cursor > b.nextSeq {
		return nil, 0, false, false, false
	}
	if cursor < b.headSeq {
		return nil, 0, false, false, false
	}
	if cursor == b.nextSeq {
		return nil, b.nextSeq, false, false, true
	}
	start := int(cursor - b.headSeq)
	if start < 0 || start > len(b.items) {
		return nil, 0, false, false, false
	}
	total := 0
	next = cursor
	for i := start; i < len(b.items); i++ {
		item := b.items[i]
		if len(items) > 0 && total+len(item.Text) > limit {
			truncated = true
			hasMore = true
			return items, next, hasMore, truncated, true
		}
		items = append(items, item)
		total += len(item.Text)
		next = item.Seq + 1
	}
	hasMore = next < b.nextSeq
	return items, next, hasMore, truncated, true
}
