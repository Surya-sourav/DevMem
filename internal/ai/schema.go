package ai

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ModuleAnalysis is the AI-generated analysis for one module.
type ModuleAnalysis struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Purpose     string   `json:"purpose"`
	KeyFiles    []string `json:"key_files"`
	PublicAPI   []string `json:"public_api"`
	DependsOn   []string `json:"depends_on"`
	TechNotes   string   `json:"tech_notes"`
	Patterns    []string `json:"patterns"`
}

// MasterAnalysis is the AI-generated architecture summary.
type MasterAnalysis struct {
	ProjectName  string          `json:"project_name"`
	Overview     string          `json:"overview"`
	TechStack    []string        `json:"tech_stack"`
	DataFlow     string          `json:"data_flow"`
	Modules      []ModuleSummary `json:"modules"`
	MermaidEdges []MermaidEdge   `json:"mermaid_edges"`
}

// ModuleSummary is one line for a module in the master doc.
type ModuleSummary struct {
	Name           string `json:"name"`
	OneLineSummary string `json:"one_line_summary"`
}

// MermaidEdge is one directional connection between modules.
type MermaidEdge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Label string `json:"label"`
}

// ChangeClassification describes one git change.
type ChangeClassification struct {
	Type     string            `json:"type"`
	Summary  string            `json:"summary"`
	Breaking bool              `json:"breaking"`
	Modules  map[string]string `json:"modules"`
}

// DocPatch contains updated doc sections only.
type DocPatch struct {
	Sections map[string]string `json:"sections"`
}

// ValidateModuleAnalysis parses and validates a module response.
func ValidateModuleAnalysis(raw string) (*ModuleAnalysis, error) {
	raw = extractJSONObject(raw)
	var out ModuleAnalysis
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("parse module analysis JSON: %w", err)
	}
	if strings.TrimSpace(out.Name) == "" {
		return nil, fmt.Errorf("module analysis missing required field: name")
	}
	if strings.TrimSpace(out.Description) == "" {
		return nil, fmt.Errorf("module analysis missing required field: description")
	}
	if strings.TrimSpace(out.Purpose) == "" {
		return nil, fmt.Errorf("module analysis missing required field: purpose")
	}
	return &out, nil
}

// ValidateMasterAnalysis parses and validates a master response.
func ValidateMasterAnalysis(raw string) (*MasterAnalysis, error) {
	raw = extractJSONObject(raw)
	var out MasterAnalysis
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("parse master analysis JSON: %w", err)
	}
	if strings.TrimSpace(out.ProjectName) == "" {
		return nil, fmt.Errorf("master analysis missing required field: project_name")
	}
	if strings.TrimSpace(out.Overview) == "" {
		return nil, fmt.Errorf("master analysis missing required field: overview")
	}
	if strings.TrimSpace(out.DataFlow) == "" {
		return nil, fmt.Errorf("master analysis missing required field: data_flow")
	}
	return &out, nil
}

// ValidateChangeClassification parses and validates a change classification response.
func ValidateChangeClassification(raw string) (*ChangeClassification, error) {
	raw = extractJSONObject(raw)
	var out ChangeClassification
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("parse change classification JSON: %w", err)
	}
	allowed := map[string]struct{}{
		"feature": {}, "fix": {}, "refactor": {}, "perf": {}, "breaking": {}, "docs": {}, "test": {}, "config": {}, "chore": {}, "structural": {},
	}
	if _, ok := allowed[out.Type]; !ok {
		return nil, fmt.Errorf("change classification type must be one of feature|fix|refactor|perf|breaking|docs|test|config|chore|structural")
	}
	if strings.TrimSpace(out.Summary) == "" {
		return nil, fmt.Errorf("change classification missing required field: summary")
	}
	if out.Modules == nil {
		out.Modules = map[string]string{}
	}
	return &out, nil
}

// ValidateDocPatch parses and validates a doc patch response.
func ValidateDocPatch(raw string) (*DocPatch, error) {
	raw = extractJSONObject(raw)
	var out DocPatch
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("parse doc patch JSON: %w", err)
	}
	if out.Sections == nil {
		return nil, fmt.Errorf("doc patch missing required field: sections")
	}
	return &out, nil
}

func extractJSONObject(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "```") {
		lines := strings.Split(trimmed, "\n")
		if len(lines) >= 2 {
			start := 1
			end := len(lines)
			if strings.HasPrefix(strings.TrimSpace(lines[0]), "```") {
				start = 1
			}
			if strings.TrimSpace(lines[len(lines)-1]) == "```" {
				end = len(lines) - 1
			}
			if start < end {
				trimmed = strings.TrimSpace(strings.Join(lines[start:end], "\n"))
			}
		}
	}

	objStart := strings.Index(trimmed, "{")
	objEnd := strings.LastIndex(trimmed, "}")
	if objStart >= 0 && objEnd > objStart {
		return strings.TrimSpace(trimmed[objStart : objEnd+1])
	}
	arrStart := strings.Index(trimmed, "[")
	arrEnd := strings.LastIndex(trimmed, "]")
	if arrStart >= 0 && arrEnd > arrStart {
		return strings.TrimSpace(trimmed[arrStart : arrEnd+1])
	}
	return trimmed
}
