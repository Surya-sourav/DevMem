package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
)

// ModulePromptInput is the context passed to module analysis prompts.
type ModulePromptInput struct {
	ModuleName   string
	FileTree     string
	EntryContent string
	SiblingNames []string
}

// MasterPromptInput is the context passed to master architecture prompts.
type MasterPromptInput struct {
	ProjectName string
	Modules     []ModuleAnalysis
}

// ChangePromptInput is the context passed to change classification prompts.
type ChangePromptInput struct {
	CommitMessage string
	DiffPatch     string
	ModuleNames   []string
}

// PatchPromptInput is the context passed to doc patch prompts.
type PatchPromptInput struct {
	CurrentDoc    string
	ChangeSummary string
	DiffPatch     string
}

var (
	moduleSystemTmpl = template.Must(template.New("module-system").Parse(`You are a senior software architecture analyst.
Return ONLY valid JSON matching this schema exactly:
{"name":"string","description":"string","purpose":"string","key_files":["string"],"public_api":["string"],"depends_on":["string"],"tech_notes":"string","patterns":["string"]}
Rules:
- Do not invent file paths that do not exist in the provided tree.
- Keep depends_on limited to sibling module names only when supported by evidence.
- Output must be strict JSON with double quotes and no markdown.`))

	moduleUserTmpl = template.Must(template.New("module-user").Parse(`Module: {{.ModuleName}}
Sibling modules: {{.SiblingCSV}}

File tree:
{{.FileTree}}

Entry file excerpt:
{{.EntryContent}}`))

	masterSystemTmpl = template.Must(template.New("master-system").Parse(`You are a principal architect creating a master system summary.
Return ONLY valid JSON matching this schema exactly:
{"project_name":"string","overview":"string","tech_stack":["string"],"data_flow":"string","modules":[{"name":"string","one_line_summary":"string"}],"mermaid_edges":[{"from":"string","to":"string","label":"string"}]}
Rules:
- Mermaid edges must be derived only from depends_on values in provided module analyses.
- Do not hallucinate dependencies.
- Output strict JSON only.`))

	masterUserTmpl = template.Must(template.New("master-user").Parse(`Project: {{.ProjectName}}
Module analyses JSON:
{{.ModulesJSON}}`))

	changeSystemTmpl = template.Must(template.New("change-system").Parse(`You classify repository changes.
Return ONLY valid JSON with schema:
{"type":"feature|fix|refactor|perf|breaking|docs|test|config|chore|structural","summary":"string","breaking":true|false,"modules":{"module":"summary"}}
Choose exactly one type enum.`))

	changeUserTmpl = template.Must(template.New("change-user").Parse(`Commit message:
{{.CommitMessage}}

Candidate modules: {{.ModuleCSV}}

Diff patch (truncated):
{{.DiffPatch}}`))

	patchSystemTmpl = template.Must(template.New("patch-system").Parse(`You update documentation surgically.
Return ONLY valid JSON with schema:
{"sections":{"Section Heading":"Updated section body"}}
Rules:
- Include only sections that changed.
- Do not rewrite entire document.
- Keep unchanged headings omitted.`))

	patchUserTmpl = template.Must(template.New("patch-user").Parse(`Current document:
{{.CurrentDoc}}

Change summary:
{{.ChangeSummary}}

Diff:
{{.DiffPatch}}`))
)

// RenderModulePrompts renders module analysis system and user prompts.
func RenderModulePrompts(input ModulePromptInput) (string, string, error) {
	system, err := renderTemplate(moduleSystemTmpl, input)
	if err != nil {
		return "", "", err
	}
	user, err := renderTemplate(moduleUserTmpl, struct {
		ModuleName   string
		SiblingCSV   string
		FileTree     string
		EntryContent string
	}{
		ModuleName:   input.ModuleName,
		SiblingCSV:   strings.Join(input.SiblingNames, ", "),
		FileTree:     input.FileTree,
		EntryContent: input.EntryContent,
	})
	if err != nil {
		return "", "", err
	}
	return system, user, nil
}

// RenderMasterPrompts renders master architecture prompts.
func RenderMasterPrompts(input MasterPromptInput) (string, string, error) {
	modulesJSON, err := toPrettyJSON(input.Modules)
	if err != nil {
		return "", "", fmt.Errorf("marshal module analyses: %w", err)
	}
	userInput := struct {
		ProjectName string
		ModulesJSON string
	}{ProjectName: input.ProjectName, ModulesJSON: modulesJSON}
	system, err := renderTemplate(masterSystemTmpl, input)
	if err != nil {
		return "", "", err
	}
	user, err := renderTemplate(masterUserTmpl, userInput)
	if err != nil {
		return "", "", err
	}
	return system, user, nil
}

// RenderChangePrompts renders change classification prompts.
func RenderChangePrompts(input ChangePromptInput) (string, string, error) {
	if len(input.DiffPatch) > 4000 {
		input.DiffPatch = input.DiffPatch[:4000]
	}
	system, err := renderTemplate(changeSystemTmpl, input)
	if err != nil {
		return "", "", err
	}
	user, err := renderTemplate(changeUserTmpl, struct {
		CommitMessage string
		ModuleCSV     string
		DiffPatch     string
	}{
		CommitMessage: input.CommitMessage,
		ModuleCSV:     strings.Join(input.ModuleNames, ", "),
		DiffPatch:     input.DiffPatch,
	})
	if err != nil {
		return "", "", err
	}
	return system, user, nil
}

// RenderPatchPrompts renders documentation patch prompts.
func RenderPatchPrompts(input PatchPromptInput) (string, string, error) {
	system, err := renderTemplate(patchSystemTmpl, input)
	if err != nil {
		return "", "", err
	}
	user, err := renderTemplate(patchUserTmpl, input)
	if err != nil {
		return "", "", err
	}
	return system, user, nil
}

func renderTemplate(t *template.Template, input any) (string, error) {
	var b bytes.Buffer
	if err := t.Execute(&b, input); err != nil {
		return "", fmt.Errorf("render template %s: %w", t.Name(), err)
	}
	return b.String(), nil
}

func toPrettyJSON(v any) (string, error) {
	b2, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b2), nil
}
