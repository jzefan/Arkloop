package firecracker

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

var firecrackerVersionPattern = regexp.MustCompile(`v?(\d+)\.(\d+)\.(\d+)`)

type Version struct {
	Major int
	Minor int
	Patch int
}

var MinSnapshotTapPatchVersion = Version{Major: 1, Minor: 12, Patch: 1}

func ParseVersion(raw string) (Version, error) {
	match := firecrackerVersionPattern.FindStringSubmatch(strings.TrimSpace(raw))
	if len(match) != 4 {
		return Version{}, fmt.Errorf("parse firecracker version from %q", raw)
	}
	major, _ := strconv.Atoi(match[1])
	minor, _ := strconv.Atoi(match[2])
	patch, _ := strconv.Atoi(match[3])
	return Version{Major: major, Minor: minor, Patch: patch}, nil
}

func (v Version) Less(other Version) bool {
	if v.Major != other.Major {
		return v.Major < other.Major
	}
	if v.Minor != other.Minor {
		return v.Minor < other.Minor
	}
	return v.Patch < other.Patch
}

func DetectVersion(ctx context.Context, bin string) (Version, error) {
	output, err := exec.CommandContext(ctx, bin, "--version").CombinedOutput()
	if err != nil {
		return Version{}, fmt.Errorf("run firecracker --version: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return ParseVersion(string(output))
}

