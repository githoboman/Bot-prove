package phase2

import "fmt"

// ValidateDAG checks a proof chain for structural correctness.
//
// Rules:
//  1. No cycles (topological sort must succeed)
//  2. Input continuity: input(step[i]) == output(parent)
//  3. Root step has no parents
//  4. All parent IDs reference existing steps
func ValidateDAG(chain *ProofChain) error {
	if len(chain.Steps) == 0 {
		return fmt.Errorf("empty chain")
	}

	index := make(map[string]*ChainStep, len(chain.Steps))
	for i := range chain.Steps {
		index[chain.Steps[i].ProofID] = &chain.Steps[i]
	}

	// Rule 3: exactly one root (no parents)
	roots := 0
	for _, step := range chain.Steps {
		if len(step.ParentIDs) == 0 {
			roots++
		}
	}
	if roots != 1 {
		return fmt.Errorf("expected 1 root, found %d", roots)
	}

	// Rule 4 + Rule 2: parent existence and input continuity
	for _, step := range chain.Steps {
		for _, pid := range step.ParentIDs {
			parent, ok := index[pid]
			if !ok {
				return fmt.Errorf("step %s references unknown parent %s", step.ProofID, pid)
			}
			if step.InputHash != parent.OutputHash {
				return fmt.Errorf(
					"input mismatch: step %d input (%s) != parent step output (%s)",
					step.StepIndex, step.InputHash[:8], parent.OutputHash[:8],
				)
			}
		}
	}

	// Rule 1: cycle detection via topological sort
	visited := make(map[string]bool)
	inStack := make(map[string]bool)
	var hasCycle bool

	var dfs func(id string)
	dfs = func(id string) {
		if hasCycle {
			return
		}
		visited[id] = true
		inStack[id] = true
		step := index[id]
		for _, pid := range step.ParentIDs {
			if inStack[pid] {
				hasCycle = true
				return
			}
			if !visited[pid] {
				dfs(pid)
			}
		}
		inStack[id] = false
	}

	for _, step := range chain.Steps {
		if !visited[step.ProofID] {
			dfs(step.ProofID)
		}
	}
	if hasCycle {
		return fmt.Errorf("cycle detected in proof chain")
	}

	return nil
}
