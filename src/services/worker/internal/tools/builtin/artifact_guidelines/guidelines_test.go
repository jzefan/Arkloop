package artifactguidelines

import (
	"strings"
	"testing"
)

func TestBuildGuidelines_ChartIncludesWidgetContractAndChartJSSource(t *testing.T) {
	got := buildGuidelines([]string{"chart"})

	for _, want := range []string{
		`title must be unique per widget`,
		`https://cdnjs.cloudflare.com/ajax/libs/Chart.js/4.4.1/chart.umd.js`,
		`responsive: true`,
		`maintainAspectRatio: false`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("guidelines missing %q\n%s", want, got)
		}
	}
}

func TestBuildGuidelines_InteractiveDoesNotRepeatSharedSections(t *testing.T) {
	got := buildGuidelines([]string{"interactive", "chart"})

	if count := strings.Count(got, "## Widget rendering contract"); count != 1 {
		t.Fatalf("expected widget contract once, got %d", count)
	}
	if count := strings.Count(got, "## Widget theme rules"); count != 1 {
		t.Fatalf("expected widget theme rules once, got %d", count)
	}
}
