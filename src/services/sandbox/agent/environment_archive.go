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
	case "browser_state":
		return []environmentRoot{{HostPath: shellHomeDir + "/.agent-browser", ArchivePath: "home/arkloop/.agent-browser"}}, nil
	case "workspace":
		return []environmentRoot{{HostPath: shellWorkspaceDir, ArchivePath: "workspace"}}, nil
	case "skills":
		return []environmentRoot{{HostPath: shellSkillsDir, ArchivePath: "opt/arkloop/skills"}}, nil
	default:
		return nil, fmt.Errorf("unsupported environment scope: %s", scope)
	}
}
