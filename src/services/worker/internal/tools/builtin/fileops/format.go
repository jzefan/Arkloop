package fileops

import (
	"fmt"
	"strings"
)

const (
	MaxReadSize      = 256 * 1024
	DefaultReadLimit = 2000
	MaxLineLength    = 2000
)

// FormatWithLineNumbers prepends right-aligned 6-char line numbers to each line.
// startLine is 1-based.
func FormatWithLineNumbers(content string, startLine int) string {
	if content == "" {
		return ""
	}
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))
	for i, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		num := i + startLine
		out = append(out, fmt.Sprintf("%6d|%s", num, line))
	}
	return strings.Join(out, "\n")
}

// TruncateLine cuts a line at maxLen characters, appending "..." if truncated.
func TruncateLine(line string, maxLen int) string {
	if len(line) <= maxLen {
		return line
	}
	return line[:maxLen] + "..."
}

// ReadLines extracts a range of lines from raw data.
// offset is 0-based (line index), limit is the max number of lines to return.
// Returns the content string, the total line count of the file, and whether
// the output was truncated (more lines exist beyond offset+limit).
func ReadLines(data []byte, offset, limit int) (content string, totalLines int, truncated bool) {
	all := strings.Split(string(data), "\n")
	totalLines = len(all)
	if offset >= totalLines {
		return "", totalLines, false
	}
	end := offset + limit
	if end > totalLines {
		end = totalLines
	}
	selected := all[offset:end]
	for i, line := range selected {
		selected[i] = TruncateLine(strings.TrimSuffix(line, "\r"), MaxLineLength)
	}
	return strings.Join(selected, "\n"), totalLines, end < totalLines
}

// CountDiffLines counts lines added and removed between old and new content.
func CountDiffLines(oldContent, newContent string) (additions, removals int) {
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")
	oldSet := make(map[string]int, len(oldLines))
	for _, l := range oldLines {
		oldSet[l]++
	}
	newSet := make(map[string]int, len(newLines))
	for _, l := range newLines {
		newSet[l]++
	}
	for l, count := range newSet {
		if oldCount, ok := oldSet[l]; ok {
			if count > oldCount {
				additions += count - oldCount
			}
		} else {
			additions += count
		}
	}
	for l, count := range oldSet {
		if newCount, ok := newSet[l]; ok {
			if count > newCount {
				removals += count - newCount
			}
		} else {
			removals += count
		}
	}
	return
}
