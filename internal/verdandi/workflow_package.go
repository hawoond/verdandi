package verdandi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type WorkflowTask struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	AgentID     string   `json:"agentId"`
	SkillIDs    []string `json:"skillIds"`
	DependsOn   []string `json:"dependsOn"`
}

type WorkflowPackage struct {
	RunID              string         `json:"runId"`
	Request            string         `json:"request"`
	Agents             []AgentAsset   `json:"agents"`
	Skills             []SkillAsset   `json:"skills"`
	Tasks              []WorkflowTask `json:"tasks"`
	AcceptanceCriteria []string       `json:"acceptanceCriteria"`
	CreatedAt          time.Time      `json:"createdAt"`
	OutputDir          string         `json:"outputDir,omitempty"`
}

type WorkflowPackageResult struct {
	OutputDir string          `json:"outputDir"`
	Files     []FileEntry     `json:"files"`
	Package   WorkflowPackage `json:"package"`
}

func WriteWorkflowPackage(baseDir string, pkg WorkflowPackage) (WorkflowPackageResult, error) {
	if pkg.CreatedAt.IsZero() {
		pkg.CreatedAt = time.Now().UTC()
	}
	if pkg.RunID == "" {
		pkg.RunID = createRunID()
	}
	outputDir := filepath.Join(baseDir, "workflows", pkg.RunID)
	if err := os.MkdirAll(filepath.Join(outputDir, "agents"), 0o755); err != nil {
		return WorkflowPackageResult{}, err
	}
	if err := os.MkdirAll(filepath.Join(outputDir, "skills"), 0o755); err != nil {
		return WorkflowPackageResult{}, err
	}
	pkg.OutputDir = outputDir

	workflowJSON, err := marshalJSON(pkg)
	if err != nil {
		return WorkflowPackageResult{}, err
	}
	selectedAssetsJSON, err := marshalJSON(map[string]any{
		"agents": pkg.Agents,
		"skills": pkg.Skills,
	})
	if err != nil {
		return WorkflowPackageResult{}, err
	}
	agentFiles := make([]generatedFile, 0, len(pkg.Agents))
	for _, agent := range pkg.Agents {
		content, err := marshalJSON(agent)
		if err != nil {
			return WorkflowPackageResult{}, err
		}
		agentFiles = append(agentFiles, generatedFile{
			Name:    safeAssetFilename(agent.ID, ".json"),
			Content: content,
		})
	}
	skillFiles := make([]generatedFile, 0, len(pkg.Skills))
	for _, skill := range pkg.Skills {
		skillFiles = append(skillFiles, generatedFile{
			Name:    safeAssetFilename(skill.ID, ".md"),
			Content: renderSkillMarkdown(skill),
		})
	}

	files := []generatedFile{
		{Name: "workflow.json", Content: workflowJSON},
		{Name: "selected-assets.json", Content: selectedAssetsJSON},
		{Name: "handoff.md", Content: renderHandoff(pkg)},
	}
	entries := make([]FileEntry, 0, len(files)+len(pkg.Agents)+len(pkg.Skills))
	for _, file := range files {
		entry, err := writeWorkflowFile(outputDir, file)
		if err != nil {
			return WorkflowPackageResult{}, err
		}
		entries = append(entries, entry)
	}
	for _, file := range agentFiles {
		entry, err := writeWorkflowFile(filepath.Join(outputDir, "agents"), file)
		if err != nil {
			return WorkflowPackageResult{}, err
		}
		entries = append(entries, entry)
	}
	for _, file := range skillFiles {
		entry, err := writeWorkflowFile(filepath.Join(outputDir, "skills"), file)
		if err != nil {
			return WorkflowPackageResult{}, err
		}
		entries = append(entries, entry)
	}

	return WorkflowPackageResult{
		OutputDir: outputDir,
		Files:     entries,
		Package:   pkg,
	}, nil
}

func writeWorkflowFile(dir string, file generatedFile) (FileEntry, error) {
	path := filepath.Join(dir, file.Name)
	mode := file.Mode
	if mode == 0 {
		mode = 0o644
	}
	if err := os.WriteFile(path, []byte(file.Content), mode); err != nil {
		return FileEntry{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return FileEntry{}, err
	}
	return FileEntry{
		Name:   file.Name,
		Path:   path,
		Size:   info.Size(),
		Status: "success",
	}, nil
}

func renderHandoff(pkg WorkflowPackage) string {
	var builder strings.Builder
	builder.WriteString("# Verdandi Workflow Handoff\n\n")
	builder.WriteString("Verdandi does not write application code. The external LLM coding agent must use the selected assets below to implement, test, and document the requested change.\n\n")
	builder.WriteString("## Request\n")
	builder.WriteString(pkg.Request)
	builder.WriteString("\n\n## Agents\n")
	for _, agent := range pkg.Agents {
		builder.WriteString("- ")
		builder.WriteString(agent.ID)
		builder.WriteString(" / ")
		builder.WriteString(agent.Role)
		builder.WriteString("\n")
	}
	builder.WriteString("\n## Skills\n")
	for _, skill := range pkg.Skills {
		builder.WriteString("- ")
		builder.WriteString(skill.ID)
		builder.WriteString(": ")
		builder.WriteString(skill.Contract.WhenToUse)
		builder.WriteString("\n")
	}
	builder.WriteString("\n## Tasks\n")
	for _, task := range pkg.Tasks {
		builder.WriteString("- ")
		builder.WriteString(task.ID)
		builder.WriteString(": ")
		builder.WriteString(task.Description)
		builder.WriteString("\n")
	}
	builder.WriteString("\n## Acceptance Criteria\n")
	for _, criterion := range pkg.AcceptanceCriteria {
		builder.WriteString("- ")
		builder.WriteString(criterion)
		builder.WriteString("\n")
	}
	return builder.String()
}

func renderSkillMarkdown(skill SkillAsset) string {
	var builder strings.Builder
	builder.WriteString("# ")
	builder.WriteString(skill.Contract.Name)
	builder.WriteString("\n\n")
	builder.WriteString(skill.Contract.Description)
	builder.WriteString("\n\n## When To Use\n")
	builder.WriteString(skill.Contract.WhenToUse)
	builder.WriteString("\n\n## Instructions\n")
	builder.WriteString(skill.Contract.Instructions)
	builder.WriteString("\n\n## Constraints\n")
	for _, constraint := range skill.Contract.Constraints {
		builder.WriteString("- ")
		builder.WriteString(constraint)
		builder.WriteString("\n")
	}
	return builder.String()
}

func marshalJSON(value any) (string, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

var unsafeAssetFilenameChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func safeAssetFilename(assetID string, ext string) string {
	name := strings.TrimSpace(assetID)
	name = strings.ReplaceAll(name, "..", "")
	name = unsafeAssetFilenameChars.ReplaceAllString(name, "-")
	name = strings.Trim(name, ".-_")
	if name == "" {
		name = "asset"
	}
	if ext != "" && !strings.HasSuffix(name, ext) {
		name += ext
	}
	return name
}
