package developer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	emperrors "emperror.dev/errors"

	"github.com/webcenter-fr/eino-ext/libs/toolkit/fileutil"
	"github.com/webcenter-fr/eino-ext/libs/toolkit/profile"
	"github.com/webcenter-fr/eino-ext/libs/toolkit/workspace"
	mcpserver "github.com/webcenter-fr/go-mcp-platform/pkg/mcp"
)

// localIdentitySegment is the workspace segment used when no authenticated
// identity is present (stdio with no LocalIdentity).
const localIdentitySegment = "local"

// identityWorkspaceRoot returns the per-identity workspace root
// <root>/<sha256(subject)[:16]>. The subject comes from the verified bearer
// token (Identity.Subject()), so it cannot be forged by the model. A nil
// identity (stdio with no LocalIdentity) maps to the "local" segment.
func identityWorkspaceRoot(root string, id *mcpserver.Identity) string {
	subject := ""
	if id != nil {
		subject = id.Subject()
	}
	if subject == "" {
		return filepath.Join(root, localIdentitySegment)
	}
	h := sha256.Sum256([]byte(subject))
	return filepath.Join(root, hex.EncodeToString(h[:8]))
}

// repoWorkspacePath returns <identityRoot>/default/<owner>/<repo>, matching the
// GitHub clone layout (<CloneDir>/<session>/<owner>/<repo> with the default
// session segment). Owner and repo are sanitized so they cannot escape the
// identity root.
func repoWorkspacePath(identityRoot, owner, repo string) string {
	return filepath.Join(
		identityRoot,
		workspace.DefaultSessionSegment,
		fileutil.SanitizePathSegment(owner, "repo"),
		fileutil.SanitizePathSegment(repo, "repo"),
	)
}

// resolveWorkspace resolves the workspace selected by owner/repo. Both empty
// selects the identity root (created on demand). A partial selection is a
// validation error; a missing repository workspace is ErrWorkspaceMissing.
func resolveWorkspace(identityRoot, owner, repo string) (string, error) {
	if (owner == "") != (repo == "") {
		return "", emperrors.New("owner and repo must be provided together")
	}
	if owner == "" && repo == "" {
		if err := os.MkdirAll(identityRoot, 0o755); err != nil {
			return "", emperrors.Wrapf(err, "failed to create identity workspace %q", identityRoot)
		}
		touchIdentityRoot(identityRoot)
		return identityRoot, nil
	}
	dir := repoWorkspacePath(identityRoot, owner, repo)
	exists, err := workspace.Exists(dir)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", emperrors.Wrapf(ErrWorkspaceMissing, "workspace %q", dir)
	}
	touchIdentityRoot(identityRoot)
	return dir, nil
}

// touchIdentityRoot refreshes the identity workspace mtime so the workspace GC
// (which sweeps <WorkspaceRoot>/<identityHash> directories older than
// WorkspaceTTL) treats it as active. Best-effort: a failed touch must not fail
// the operation.
func touchIdentityRoot(identityRoot string) {
	_ = fileutil.TouchDir(identityRoot)
}

// envKeyPattern is the accepted syntax for an environment variable name.
var envKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// envDeniedPattern rejects credential-shaped names.
var envDeniedPattern = regexp.MustCompile(`(?i)(token|secret|password|passwd|credential|apikey|api_key|private_key)`)

// envDeniedExact is the case-insensitive exact-match denylist.
var envDeniedExact = map[string]bool{
	"GITHUB_TOKEN": true, "GH_TOKEN": true,
	"KUBECONFIG": true,
	"HTTP_PROXY": true, "HTTPS_PROXY": true, "NO_PROXY": true,
	"LD_PRELOAD": true, "LD_LIBRARY_PATH": true, "LD_AUDIT": true,
	"PATH": true,
}

// envDeniedPrefixes is the case-insensitive prefix denylist.
var envDeniedPrefixes = []string{
	"GITHUB_", "KUBERNETES_", "AWS_", "AZURE_", "GOOGLE_", "GCP_",
	"DAGGER_", "_EXPERIMENTAL_DAGGER_",
}

// validateEnv rejects unsafe environment variable names and values.
func validateEnv(env map[string]string) error {
	for key, value := range env {
		if !envKeyPattern.MatchString(key) {
			return emperrors.Wrapf(ErrEnvRejected, "key %q must match %s", key, envKeyPattern.String())
		}
		upper := strings.ToUpper(key)
		if envDeniedExact[upper] {
			return emperrors.Wrapf(ErrEnvRejected, "key %q is reserved", key)
		}
		for _, prefix := range envDeniedPrefixes {
			if strings.HasPrefix(upper, prefix) {
				return emperrors.Wrapf(ErrEnvRejected, "key %q uses a reserved prefix %q", key, prefix)
			}
		}
		if envDeniedPattern.MatchString(key) {
			return emperrors.Wrapf(ErrEnvRejected, "key %q looks credential-shaped", key)
		}
		if strings.ContainsRune(value, '\x00') {
			return emperrors.Wrapf(ErrEnvRejected, "value for key %q contains a NUL byte", key)
		}
		if len(value) > 4096 {
			return emperrors.Wrapf(ErrEnvRejected, "value for key %q exceeds 4096 bytes", key)
		}
	}
	return nil
}

// resolveImage detects the project profile under workdir and returns the
// profile name and base image. The fallback profile uses cfg.BaseImage.
func resolveImage(ctx context.Context, cfg *Config, resolver *profile.Resolver, workdir string) (string, string, error) {
	profiles, err := resolver.Resolve(ctx, workdir)
	if err != nil {
		return "", "", emperrors.Wrap(err, "failed to detect project profile")
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].Name < profiles[j].Name })
	selected := profiles[0]
	image := selected.BaseImage
	if selected.Name == "default" {
		image = cfg.BaseImage
	}
	if len(cfg.AllowedImages) > 0 && !slices.Contains(cfg.AllowedImages, image) {
		return "", "", emperrors.Wrapf(ErrImageNotAllowed, "image %q (profile %q)", image, selected.Name)
	}
	return selected.Name, image, nil
}

// truncateOutput caps stdout and stderr. It keeps the tail of stdout and the
// head of stderr, inserting an explicit omission marker.
func truncateOutput(stdout, stderr string, maxBytes int) (string, string, bool) {
	if maxBytes <= 0 {
		return stdout, stderr, false
	}
	half := maxBytes / 2
	truncated := false
	if len(stdout) > half {
		omitted := len(stdout) - half
		stdout = fmt.Sprintf("[... %d bytes omitted ...]\n%s", omitted, stdout[len(stdout)-half:])
		truncated = true
	}
	if len(stderr) > half {
		omitted := len(stderr) - half
		stderr = fmt.Sprintf("%s\n[... %d bytes omitted ...]", stderr[:half], omitted)
		truncated = true
	}
	return stdout, stderr, truncated
}

// compileFilter compiles a per-line RE2 filter. An empty pattern returns nil.
func compileFilter(pattern string) (*regexp.Regexp, error) {
	if pattern == "" {
		return nil, nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, emperrors.Wrapf(err, "invalid filterPattern %q", pattern)
	}
	return re, nil
}

// filterOutput applies a compiled per-line filter to s. A nil filter returns s.
func filterOutput(s string, re *regexp.Regexp) string {
	if re == nil {
		return s
	}
	lines := strings.Split(s, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if re.MatchString(line) {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}
