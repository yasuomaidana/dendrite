package triage

import (
	"fmt"

	"Dendrite/pkg/rag"
)

// SynthesizeDeclarativePlan generates a concise set of declarative instructions as a cloud orchestrator would,
// grounded by retrieved RAG architecture chunks and AST constraints.
func (e *Engine) SynthesizeDeclarativePlan(prompt, activeFile string, skills []string, ragChunks []rag.Chunk) *DeclarativePlan {
	var targetFiles []string
	if activeFile != "" {
		targetFiles = append(targetFiles, activeFile)
	}

	for _, c := range ragChunks {
		if c.FilePath != "" && c.FilePath != activeFile {
			found := false
			for _, tf := range targetFiles {
				if tf == c.FilePath {
					found = true
					break
				}
			}
			if !found && len(targetFiles) < 4 {
				targetFiles = append(targetFiles, c.FilePath)
			}
		}
	}

	instructions := []string{
		fmt.Sprintf("1. Decompose requirement into atomic functions matching: %s", prompt),
	}
	if len(ragChunks) > 0 && ragChunks[0].FilePath != "" {
		instructions = append(instructions, fmt.Sprintf("2. Respect existing architectural patterns and types from %s", ragChunks[0].FilePath))
		instructions = append(instructions, "3. Implement core interfaces, structs, and boundary error handling.")
		instructions = append(instructions, "4. Author comprehensive unit test suite covering happy paths and edge cases.")
		instructions = append(instructions, "5. Verify zero compiler warnings and run static analysis.")
	} else {
		instructions = append(instructions, "2. Implement core interfaces, structs, and boundary error handling.")
		instructions = append(instructions, "3. Author comprehensive unit test suite covering happy paths and edge cases.")
		instructions = append(instructions, "4. Verify zero compiler warnings and run static analysis.")
	}

	constraints := []string{
		"Cyclomatic complexity <= 15 per function",
		"Zero external dependencies outside allowed stdlib and packages",
	}

	for _, s := range skills {
		switch s {
		case "ticket-lifecycle-developer":
			constraints = append(constraints, "Enforce Plane ticket traceability and statement test coverage >= 85%")
		case "pattern-complexity-auditor":
			constraints = append(constraints, "Enforce cognitive complexity <= 20 and idiomatic interface composition")
		case "crate-compatibility":
			constraints = append(constraints, "Validate lockfile compatibility and semantic version constraints")
		}
	}

	// Calculate token footprint
	estOrchTokens := 110 + len(targetFiles)*10
	estDirectTokens := 1600
	savingsPct := (1.0 - float64(estOrchTokens)/float64(estDirectTokens)) * 100.0

	return &DeclarativePlan{
		Goal:                        prompt,
		TargetFiles:                 targetFiles,
		Instructions:                instructions,
		Constraints:                 constraints,
		EstimatedOrchestratorTokens: estOrchTokens,
		EstimatedDirectOutputTokens: estDirectTokens,
		OutputTokenSavingsPct:       savingsPct,
	}
}
