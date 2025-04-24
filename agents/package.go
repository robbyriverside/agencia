package agents

import (
	"context"
	"fmt"
	"strings"
)

func (a Agent) Call(ctx context.Context, input map[string]any) (string, error) {
	if a.Function != nil {
		return a.Function(ctx, input)
	}
	return "", fmt.Errorf("agent has no function: %s", a.Description)
}

// PackageRepo holds all available agent packages
type PackageRepo struct {
	Packages map[string]map[string]Agent
	Working  map[string]Agent // working package (agentRegistry)
}

// LookupAgent resolves both unqualified and qualified agent names
func (r *PackageRepo) LookupAgent(name string) (Agent, error) {
	if !strings.Contains(name, ".") {
		agent, ok := r.Working[name]
		if !ok {
			return Agent{}, fmt.Errorf("agent not found in working package: %s", name)
		}
		return agent, nil
	}

	parts := strings.SplitN(name, ".", 2)
	pkgName, agentName := parts[0], parts[1]
	pkg, ok := r.Packages[pkgName]
	if !ok {
		return Agent{}, fmt.Errorf("unknown package: %s", pkgName)
	}
	agent, ok := pkg[agentName]
	if !ok {
		return Agent{}, fmt.Errorf("agent %s not found in package %s", name, pkgName)
	}
	return agent, nil
}
