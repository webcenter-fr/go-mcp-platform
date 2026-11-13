package developer

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTemplatesAreComplete(t *testing.T) {
	templates := Templates()
	require.NotEmpty(t, templates)
	seen := map[string]bool{}
	for _, tmpl := range templates {
		assert.NotEmpty(t, tmpl.ID)
		assert.NotEmpty(t, tmpl.Title)
		assert.NotEmpty(t, tmpl.TargetPath)
		assert.NotEmpty(t, tmpl.MIMEType)
		assert.NotEmpty(t, tmpl.Body)
		assert.False(t, seen[tmpl.ID], "duplicate template id %q", tmpl.ID)
		seen[tmpl.ID] = true
	}
	for _, stage := range []LifecycleStage{
		StageArchitect, StageDevelop, StageReview, StageSecurity, StageDeepSecurity, StageDeepBugSearch,
	} {
		assert.NotEmpty(t, lifecyclePrompts[stage], "missing prompt for stage %q", stage)
		assert.NotEmpty(t, lifecycleTiers[stage], "missing tier for stage %q", stage)
	}
}

func TestScaffoldListFilters(t *testing.T) {
	svc := &scaffoldService{identityRoot: t.TempDir()}
	out, err := svc.list(context.Background(), &ScaffoldListParams{Kind: "agents", Language: "golang"})
	require.NoError(t, err)
	var result ScaffoldListOutput
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Len(t, result.Templates, 1)
	assert.Equal(t, "agents-golang", result.Templates[0].ID)
}

func TestScaffoldGetUnknown(t *testing.T) {
	svc := &scaffoldService{identityRoot: t.TempDir()}
	_, err := svc.get(context.Background(), &ScaffoldGetParams{ID: "missing"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTemplateNotFound)
}

func TestScaffoldApply(t *testing.T) {
	identityRoot := t.TempDir()
	svc := &scaffoldService{identityRoot: identityRoot}
	repoDir := repoWorkspacePath(identityRoot, "owner", "repo")

	out, err := svc.apply(context.Background(), &ScaffoldApplyParams{
		IDs:   []string{"agents-golang", "opencode-config"},
		Owner: "owner",
		Repo:  "repo",
	})
	require.NoError(t, err)
	var result ScaffoldApplyOutput
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.ElementsMatch(t, []string{"agents-golang", "opencode-config"}, result.Written)

	agentsPath := filepath.Join(repoDir, "AGENTS.md")
	info, err := os.Stat(agentsPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())

	// Existing file is skipped when overwrite is false.
	require.NoError(t, os.WriteFile(agentsPath, []byte("custom"), 0o644))
	out, err = svc.apply(context.Background(), &ScaffoldApplyParams{
		IDs:   []string{"agents-golang"},
		Owner: "owner",
		Repo:  "repo",
	})
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.Equal(t, []string{"agents-golang"}, result.Skipped)
	content, err := os.ReadFile(agentsPath)
	require.NoError(t, err)
	assert.Equal(t, "custom", string(content))

	// Overwrite replaces it.
	out, err = svc.apply(context.Background(), &ScaffoldApplyParams{
		IDs:       []string{"agents-golang"},
		Owner:     "owner",
		Repo:      "repo",
		Overwrite: true,
	})
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.Equal(t, []string{"agents-golang"}, result.Written)
	content, err = os.ReadFile(agentsPath)
	require.NoError(t, err)
	assert.NotEqual(t, "custom", string(content))
}

func TestScaffoldApplyDryRunWritesNothing(t *testing.T) {
	identityRoot := t.TempDir()
	svc := &scaffoldService{identityRoot: identityRoot}
	repoDir := repoWorkspacePath(identityRoot, "owner", "repo")

	out, err := svc.apply(context.Background(), &ScaffoldApplyParams{
		IDs:    []string{"agents-golang"},
		Owner:  "owner",
		Repo:   "repo",
		DryRun: true,
	})
	require.NoError(t, err)
	var result ScaffoldApplyOutput
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	assert.True(t, result.DryRun)
	assert.Equal(t, []string{"agents-golang"}, result.Written)
	_, err = os.Stat(filepath.Join(repoDir, "AGENTS.md"))
	assert.True(t, os.IsNotExist(err))
}

func TestApplyTemplatesRejectsTraversal(t *testing.T) {
	repoDir := t.TempDir()
	_, err := applyTemplates(repoDir, []Template{{
		ID:         "malicious",
		TargetPath: "../escape.md",
		Body:       "x",
	}}, false, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid target path")
}

func TestScaffoldPromptsAndResources(t *testing.T) {
	prompts := scaffoldPrompts()
	require.Len(t, prompts, 4)
	names := make([]string, 0, len(prompts))
	for _, p := range prompts {
		names = append(names, p.Name)
	}
	assert.ElementsMatch(t, []string{"developer_bootstrap", "developer_review", "developer_security_audit", "developer_lifecycle"}, names)

	resources := scaffoldResources()
	uris := make([]string, 0, len(resources))
	for _, r := range resources {
		uris = append(uris, r.URI)
	}
	assert.Contains(t, uris, "developer://templates")
	assert.Contains(t, uris, "developer://agents/golang")
	assert.Contains(t, uris, "developer://lifecycle/workflow")
	assert.Contains(t, uris, "developer://opencode/config")
}
