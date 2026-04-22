package edit

import (
	"fmt"
	"strings"
)

// unifiedDiff generates a standard unified diff between oldContent and newContent.
// filePath is used in the --- / +++ header lines.
func unifiedDiff(oldContent, newContent, filePath string) string {
	if oldContent == newContent {
		return ""
	}

	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	hunks := computeHunks(oldLines, newLines, 3)
	if len(hunks) == 0 {
		return ""
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "--- a/%s\n", filePath)
	fmt.Fprintf(&sb, "+++ b/%s\n", filePath)
	for _, h := range hunks {
		sb.WriteString(h)
	}
	return sb.String()
}

type edit struct {
	oldStart, oldLen int
	newStart, newLen int
	lines            []string // lines prefixed with ' ', '-', '+'
}

// computeHunks uses Myers-style LCS to build unified diff hunks with `context` lines of context.
func computeHunks(oldLines, newLines []string, context int) []string {
	ops := lcs(oldLines, newLines)

	// group ops into hunks
	var hunks []string
	i := 0
	for i < len(ops) {
		// skip equal ops until we find a change
		for i < len(ops) && ops[i].kind == '=' {
			i++
		}
		if i >= len(ops) {
			break
		}

		// find the extent of this change cluster (including context)
		start := i - context
		if start < 0 {
			start = 0
		}
		// extend forward: collect all changes + context
		end := i
		for end < len(ops) {
			if ops[end].kind != '=' {
				end++
				continue
			}
			// count consecutive equal lines after a change
			gap := 0
			j := end
			for j < len(ops) && ops[j].kind == '=' {
				gap++
				j++
			}
			if j < len(ops) && gap <= 2*context {
				// merge into this hunk
				end = j
			} else {
				break
			}
		}
		end += context
		if end > len(ops) {
			end = len(ops)
		}

		chunk := ops[start:end]
		var oldStart, oldCount, newStart, newCount int
		var lines []string
		for k, op := range chunk {
			switch op.kind {
			case '=':
				lines = append(lines, " "+op.text)
				oldCount++
				newCount++
				if k == 0 {
					oldStart = op.oldIdx + 1
					newStart = op.newIdx + 1
				}
			case '-':
				lines = append(lines, "-"+op.text)
				oldCount++
				if oldStart == 0 {
					oldStart = op.oldIdx + 1
					newStart = op.newIdx + 1
				}
			case '+':
				lines = append(lines, "+"+op.text)
				newCount++
				if oldStart == 0 {
					oldStart = op.oldIdx + 1
					newStart = op.newIdx + 1
				}
			}
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "@@ -%d,%d +%d,%d @@\n", oldStart, oldCount, newStart, newCount)
		for _, l := range lines {
			sb.WriteString(l)
			sb.WriteByte('\n')
		}
		hunks = append(hunks, sb.String())
		i = end
	}
	return hunks
}

type op struct {
	kind   byte // '=', '-', '+'
	text   string
	oldIdx int
	newIdx int
}

// lcs computes the edit script via Myers diff (O(ND) greedy).
func lcs(a, b []string) []op {
	n, m := len(a), len(b)
	if n == 0 && m == 0 {
		return nil
	}
	max := n + m
	v := make([]int, 2*max+1)
	type point struct{ x, y int }
	trace := make([][]int, 0, max+1)

	for d := 0; d <= max; d++ {
		snap := make([]int, len(v))
		copy(snap, v)
		trace = append(trace, snap)

		for k := -d; k <= d; k += 2 {
			var x int
			ki := k + max
			if k == -d || (k != d && v[ki-1] < v[ki+1]) {
				x = v[ki+1]
			} else {
				x = v[ki-1] + 1
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			v[ki] = x
			if x >= n && y >= m {
				return backtrack(a, b, trace, d, max)
			}
		}
	}
	return backtrack(a, b, trace, max, max)
}

func backtrack(a, b []string, trace [][]int, d, offset int) []op {
	x, y := len(a), len(b)
	var ops []op
	for dd := d; dd > 0; dd-- {
		v := trace[dd]
		k := x - y
		ki := k + offset
		var prevK int
		if k == -dd || (k != dd && v[ki-1] < v[ki+1]) {
			prevK = k + 1
		} else {
			prevK = k - 1
		}
		prevX := v[prevK+offset]
		prevY := prevX - prevK

		// diagonal snake
		for x > prevX && y > prevY {
			x--
			y--
			ops = append(ops, op{'=', a[x], x, y})
		}
		if dd > 0 {
			if x == prevX {
				y--
				ops = append(ops, op{'+', b[y], x, y})
			} else {
				x--
				ops = append(ops, op{'-', a[x], x, y})
			}
		}
		x, y = prevX, prevY
	}
	// remaining diagonal at d==0
	for x > 0 && y > 0 {
		x--
		y--
		ops = append(ops, op{'=', a[x], x, y})
	}

	// reverse
	for i, j := 0, len(ops)-1; i < j; i, j = i+1, j-1 {
		ops[i], ops[j] = ops[j], ops[i]
	}
	return ops
}
