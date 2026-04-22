package fileops

import (
	"testing"
)

func TestDetectOmissionInContent(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name:    "clean code",
			content: "func main() {\n\tfmt.Println(\"hello\")\n}",
			wantErr: false,
		},
		{
			name:    "rest of code with double-slash comment",
			content: "func main() {\n\t// rest of code...\n}",
			wantErr: true,
		},
		{
			name:    "existing code with hash comment",
			content: "def main():\n    # existing code...\n    pass",
			wantErr: true,
		},
		{
			name:    "unicode ellipsis",
			content: "func main() {\n\t// rest of file…\n}",
			wantErr: true,
		},
		{
			name:    "no ellipsis means safe",
			content: "// rest of code is fine without dots",
			wantErr: false,
		},
		{
			name:    "html comment",
			content: "<!-- remaining code... -->",
			wantErr: true,
		},
		{
			name:    "legitimate triple dots in string",
			content: `msg := "loading..."`,
			wantErr: false,
		},
		{
			name:    "keep existing pattern",
			content: "/* keep existing... */",
			wantErr: true,
		},
		{
			name:    "empty content",
			content: "",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := DetectOmissionInContent(tt.content)
			if (err != nil) != tt.wantErr {
				t.Errorf("DetectOmissionInContent() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
