package developer

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mcpserver "github.com/webcenter-fr/go-mcp-platform/pkg/mcp"
)

func TestIdentityWorkspaceRoot(t *testing.T) {
	root := t.TempDir()
	a := identityWorkspaceRoot(root, &mcpserver.Identity{User: "alice", Sub: "sub-a"})
	b := identityWorkspaceRoot(root, &mcpserver.Identity{User: "bob", Sub: "sub-b"})

	assert.NotEqual(t, a, b)
	assert.Equal(t, a, identityWorkspaceRoot(root, &mcpserver.Identity{User: "alice", Sub: "sub-a"}), "must be stable")
	assert.Regexp(t, regexp.MustCompile(`^[0-9a-f]{16}$`), filepath.Base(a))
	assert.Equal(t, filepath.Join(root, localIdentitySegment), identityWorkspaceRoot(root, nil))
	assert.Equal(t, filepath.Join(root, localIdentitySegment), identityWorkspaceRoot(root, &mcpserver.Identity{}))
}

func TestRepoWorkspacePathIsPerIdentity(t *testing.T) {
	root := t.TempDir()
	a := identityWorkspaceRoot(root, &mcpserver.Identity{Sub: "sub-a"})
	b := identityWorkspaceRoot(root, &mcpserver.Identity{Sub: "sub-b"})
	pathA := repoWorkspacePath(a, "owner", "repo")
	pathB := repoWorkspacePath(b, "owner", "repo")
	assert.NotEqual(t, pathA, pathB)
	assert.Contains(t, pathA, filepath.Join("default", "owner", "repo"))
}

func TestResolveWorkspace(t *testing.T) {
	root := t.TempDir()
	repoDir := repoWorkspacePath(root, "owner", "repo")
	require.NoError(t, os.MkdirAll(repoDir, 0o755))

	got, err := resolveWorkspace(root, "owner", "repo")
	require.NoError(t, err)
	assert.Equal(t, repoDir, got)

	_, err = resolveWorkspace(root, "owner", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "together")

	_, err = resolveWorkspace(root, "owner", "missing")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrWorkspaceMissing)

	got, err = resolveWorkspace(root, "", "")
	require.NoError(t, err)
	assert.Equal(t, root, got)
}

func TestResolveWorkspaceTouchesIdentityRoot(t *testing.T) {
	// The workspace GC sweeps <WorkspaceRoot>/<identityHash> directories older
	// than WorkspaceTTL. resolveWorkspace must refresh the identity root mtime
	// so an actively-used workspace is not swept.
	root := t.TempDir()
	old := time.Now().Add(-2 * time.Hour)
	require.NoError(t, os.Chtimes(root, old, old))

	_, err := resolveWorkspace(root, "", "")
	require.NoError(t, err)

	fi, err := os.Stat(root)
	require.NoError(t, err)
	assert.True(t, fi.ModTime().After(old.Add(time.Minute)), "identity root mtime must be refreshed on activity")
}

func TestValidateEnv(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr bool
	}{
		{name: "safe", env: map[string]string{"ok_VAR": "value"}},
		{name: "github token", env: map[string]string{"GITHUB_TOKEN": "x"}, wantErr: true},
		{name: "kubeconfig", env: map[string]string{"kubeconfig": "x"}, wantErr: true},
		{name: "ld preload", env: map[string]string{"LD_PRELOAD": "x"}, wantErr: true},
		{name: "path", env: map[string]string{"PATH": "x"}, wantErr: true},
		{name: "credential shaped", env: map[string]string{"MY_SECRET": "x"}, wantErr: true},
		{name: "proxy", env: map[string]string{"HTTP_PROXY": "x"}, wantErr: true},
		{name: "bad key syntax", env: map[string]string{"1BAD": "x"}, wantErr: true},
		{name: "nul value", env: map[string]string{"OK": "a\x00b"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEnv(tt.env)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrEnvRejected)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestResolveWorkdirRejectsTraversalAbsoluteAndSymlink(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "sub"), 0o755))
	outside := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "link")))

	tests := []struct {
		name    string
		workdir string
		wantErr bool
	}{
		{name: "valid", workdir: "sub"},
		{name: "traversal", workdir: "../../etc", wantErr: true},
		{name: "absolute", workdir: "/etc", wantErr: true},
		{name: "symlink", workdir: "link", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolveWorkdir(root, tt.workdir)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestTruncateOutput(t *testing.T) {
	stdout := "0123456789"
	stderr := "abcdefghij"
	out, errOut, truncated := truncateOutput(stdout, stderr, 10)
	assert.True(t, truncated)
	assert.Contains(t, out, "bytes omitted")
	assert.Contains(t, errOut, "bytes omitted")
	assert.Contains(t, out, "56789", "stdout keeps the tail")
	assert.Contains(t, errOut, "abcde", "stderr keeps the head")
}

func TestFilterOutput(t *testing.T) {
	re, err := compileFilter("keep")
	require.NoError(t, err)
	assert.Equal(t, "keep\nkeep2", filterOutput("keep\ndrop\nkeep2", re))

	_, err = compileFilter("(?=lookahead)")
	require.Error(t, err)
}
