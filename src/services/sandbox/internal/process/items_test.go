package process

import (
	"strings"
	"testing"
)

func TestItemBufferSplitsHugeSingleChunkAndPreservesOrder(t *testing.T) {
	buffer := NewItemBuffer(defaultItemBufferBytes)
	huge := strings.Repeat("x", maxItemChunkBytes*3+17)

	buffer.Append(StreamStdout, huge)

	items, next, hasMore, truncated, ok := buffer.ReadFrom(0, len(huge)+1024)
	if !ok {
		t.Fatal("expected read to succeed")
	}
	if truncated {
		t.Fatalf("did not expect truncation, got items=%d", len(items))
	}
	if hasMore {
		t.Fatal("did not expect more items after full read")
	}
	if next != buffer.NextSeq() {
		t.Fatalf("expected next cursor %d, got %d", buffer.NextSeq(), next)
	}
	if len(items) != 4 {
		t.Fatalf("expected 4 items after chunk split, got %d", len(items))
	}
	for i, item := range items[:3] {
		if len(item.Text) != maxItemChunkBytes {
			t.Fatalf("expected chunk %d size %d, got %d", i, maxItemChunkBytes, len(item.Text))
		}
	}
	if got := len(items[3].Text); got != 17 {
		t.Fatalf("expected final chunk size 17, got %d", got)
	}
}

func TestItemBufferExpiresOldCursorAfterOverflow(t *testing.T) {
	buffer := NewItemBuffer(maxItemChunkBytes * 2)

	buffer.Append(StreamStdout, strings.Repeat("a", maxItemChunkBytes))
	buffer.Append(StreamStdout, strings.Repeat("b", maxItemChunkBytes))
	buffer.Append(StreamStdout, strings.Repeat("c", maxItemChunkBytes))

	if buffer.HeadSeq() == 0 {
		t.Fatalf("expected head seq to advance after overflow, got %d", buffer.HeadSeq())
	}
	if _, _, _, _, ok := buffer.ReadFrom(0, defaultResponseBytes); ok {
		t.Fatal("expected cursor 0 to expire after overflow")
	}
	items, next, _, _, ok := buffer.ReadFrom(buffer.HeadSeq(), defaultResponseBytes)
	if !ok {
		t.Fatal("expected read from head cursor to succeed")
	}
	if len(items) == 0 {
		t.Fatal("expected retained items from head cursor")
	}
	if next <= buffer.HeadSeq() {
		t.Fatalf("expected next cursor to advance, got head=%d next=%d", buffer.HeadSeq(), next)
	}
}
