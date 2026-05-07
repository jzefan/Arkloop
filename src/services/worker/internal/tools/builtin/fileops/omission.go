package fileops

import (
	"fmt"
	"strings"
)

// omission placeholder phrases (lowercase, prefix-matched)
var omissionPhrases = []string{
	"rest of method",
	"rest of methods",
	"rest of code",
	"rest of file",
	"rest of function",
	"rest of class",
	"rest of",
	"unchanged code",
	"existing code",
	"previous code",
	"remaining code",
	"other methods",
	"other cases",
	"same as before",
	"keep existing",
	"no changes",
}

// comment prefixes to strip before checking
var commentPrefixes = []string{
	"<!--", "//", "#", "--", "/*", "*",
}

// DetectOmissionInContent scans content for lazy placeholder patterns
// like "// rest of code..." without an old-string reference.
// Returns nil if no placeholders found, otherwise an error describing the match.
func DetectOmissionInContent(content string) error {
	for i, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		stripped := trimmed
		for _, p := range commentPrefixes {
			if strings.HasPrefix(stripped, p) {
				stripped = strings.TrimSpace(stripped[len(p):])
				break
			}
		}

		dotIdx := strings.Index(stripped, "...")
		if dotIdx < 0 {
			dotIdx = strings.Index(stripped, "…") // unicode ellipsis
		}
		if dotIdx < 0 {
			continue
		}

		prefix := strings.ToLower(strings.TrimSpace(stripped[:dotIdx]))
		if matchesOmissionPhrase(prefix) {
			return fmt.Errorf("detected omission placeholder: '%s' at line %d — write the complete code instead of using placeholders", trimmed, i+1)
		}
	}
	return nil
}

func matchesOmissionPhrase(text string) bool {
	for _, phrase := range omissionPhrases {
		if strings.HasPrefix(text, phrase) {
			return true
		}
	}
	return false
}
