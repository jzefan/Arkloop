package main

import "fmt"

type environmentRoot struct {
	HostPath    string
	ArchivePath string
}

func environmentRoots(scope string) ([]environmentRoot, error) {
	switch scope {
	case "profile":
		return []environmentRoot{{HostPath: shellHomeDir, ArchivePath: "home/arkloop"}}, nil
	case "workspace":
		return []environmentRoot{{HostPath: shellWorkspaceDir, ArchivePath: "workspace"}}, nil
	default:
		return nil, fmt.Errorf("unsupported environment scope: %s", scope)
	}
}
