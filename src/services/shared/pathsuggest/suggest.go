package pathsuggest

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// skip these directories during traversal
var skipDirs = map[string]struct{}{
	"node_modules": {},
	".git":         {},
	"vendor":       {},
	".next":        {},
	"dist":         {},
	"build":        {},
	"__pycache__":  {},
	".cache":       {},
}

// common synonyms: each pair is bidirectional
var synonymPairs = [][2]string{
	{"src", "source"},
	{"lib", "libs"},
	{"util", "utils"},
	{"index", "main"},
	{"config", "configs"},
	{"test", "tests"},
	{"spec", "specs"},
	{"doc", "docs"},
}

const (
	maxDepth   = 4
	maxEntries = 1000
	maxResults = 3
)

// SuggestSimilarPaths returns up to 3 suggestions for a path that doesn't exist.
func SuggestSimilarPaths(targetPath string, workdir string) []string {
	if workdir == "" || targetPath == "" {
		return nil
	}

	targetPath = filepath.Clean(targetPath)
	workdir = filepath.Clean(workdir)

	// collect candidate paths
	candidates := collectCandidates(workdir)
	if len(candidates) == 0 {
		return nil
	}

	type scored struct {
		path string
		dist int
	}

	targetRel := targetPath
	if rel, err := filepath.Rel(workdir, targetPath); err == nil {
		targetRel = rel
	}
	targetRelLower := strings.ToLower(targetRel)

	var results []scored
	for _, c := range candidates {
		rel, err := filepath.Rel(workdir, c)
		if err != nil {
			continue
		}
		relLower := strings.ToLower(rel)

		// strategy 1: case-insensitive exact match
		if relLower == targetRelLower && c != targetPath {
			results = append(results, scored{c, 0})
			continue
		}

		// strategy 2: synonym substitution
		if synonymMatch(targetRelLower, relLower) {
			results = append(results, scored{c, 1})
			continue
		}

		// strategy 3: missing/extra directory layer
		if layerMatch(targetRelLower, relLower) {
			results = append(results, scored{c, 2})
			continue
		}

		// strategy 4: edit distance on relative path (low threshold)
		d := editDistance(targetRelLower, relLower)
		maxDist := len(targetRelLower) / 4
		if maxDist < 2 {
			maxDist = 2
		}
		if maxDist > 6 {
			maxDist = 6
		}
		if d > 0 && d <= maxDist {
			results = append(results, scored{c, d + 2})
		}
	}

	// also check: target might be absolute but belongs under workdir with a dropped prefix

	sort.Slice(results, func(i, j int) bool {
		return results[i].dist < results[j].dist
	})

	seen := map[string]struct{}{}
	var out []string
	for _, r := range results {
		if _, ok := seen[r.path]; ok {
			continue
		}
		seen[r.path] = struct{}{}
		out = append(out, r.path)
		if len(out) >= maxResults {
			break
		}
	}
	return out
}

func collectCandidates(workdir string) []string {
	var candidates []string
	count := 0
	_ = filepath.WalkDir(workdir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			rel, _ := filepath.Rel(workdir, path)
			depth := strings.Count(rel, string(filepath.Separator))
			if depth > maxDepth {
				return filepath.SkipDir
			}
			if _, skip := skipDirs[d.Name()]; skip && path != workdir {
				return filepath.SkipDir
			}
		}
		count++
		if count > maxEntries {
			return filepath.SkipAll
		}
		candidates = append(candidates, path)
		return nil
	})
	return candidates
}

// synonymMatch checks if one path can become the other by replacing a synonym segment.
func synonymMatch(a, b string) bool {
	partsA := strings.Split(a, string(filepath.Separator))
	partsB := strings.Split(b, string(filepath.Separator))
	if len(partsA) != len(partsB) {
		return false
	}
	diffs := 0
	for i := range partsA {
		if partsA[i] != partsB[i] {
			diffs++
			if diffs > 1 {
				return false
			}
			if !areSynonyms(partsA[i], partsB[i]) {
				return false
			}
		}
	}
	return diffs == 1
}

func areSynonyms(a, b string) bool {
	for _, pair := range synonymPairs {
		if (a == pair[0] && b == pair[1]) || (a == pair[1] && b == pair[0]) {
			return true
		}
	}
	return false
}

// layerMatch checks if one path is the other with one directory segment added or removed.
func layerMatch(target, candidate string) bool {
	tParts := strings.Split(target, string(filepath.Separator))
	cParts := strings.Split(candidate, string(filepath.Separator))

	diff := len(cParts) - len(tParts)
	if diff != 1 && diff != -1 {
		return false
	}

	// the longer path should contain all segments of the shorter in order
	shorter, longer := tParts, cParts
	if diff == -1 {
		shorter, longer = cParts, tParts
	}

	j := 0
	for i := 0; i < len(longer) && j < len(shorter); i++ {
		if longer[i] == shorter[j] {
			j++
		}
	}
	return j == len(shorter)
}

func editDistance(a, b string) int {
	ra := []rune(a)
	rb := []rune(b)
	la, lb := len(ra), len(rb)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	// bail if difference is too large
	if la-lb > 6 || lb-la > 6 {
		return la + lb
	}

	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if unicode.ToLower(ra[i-1]) == unicode.ToLower(rb[j-1]) {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			curr[j] = min3(del, ins, sub)
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
