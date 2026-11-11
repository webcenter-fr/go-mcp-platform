package main

import (
	"context"
	"fmt"

	"dagger/mcp-platform/internal/dagger"
)

const chartPath = "helm/go-mcp-platform"

// HelmLint lints the chart.
func (m *McpPlatform) HelmLint(ctx context.Context) (string, error) {
	return dag.Helm(m.Src).WithWorkDir(chartPath).Lint(ctx)
}

// HelmSchema generates values.schema.json.
func (m *McpPlatform) HelmSchema(ctx context.Context) *dagger.File {
	return dag.Helm(m.Src).WithWorkDir(chartPath).GenerateSchema()
}

// HelmDocs generates the chart README.md from a fresh header. The upstream
// GenerateDocumentation appends to an existing README, which is not idempotent.
func (m *McpPlatform) HelmDocs(ctx context.Context) *dagger.File {
	header := "# go-mcp-platform Helm chart\n\nDeploys the go-mcp-platform MCP servers and the standalone approval service.\n\n## Parameters\n\n"
	return dag.Helm(m.Src).WithWorkDir(chartPath).GeneratorContainer().
		WithExec([]string{"sh", "-c", "printf '" + header + "' > README.md && readme-generator readme -v values.yaml -r README.md"}).
		File("README.md")
}

// HelmTemplate renders the default values and every fixture, validates them
// with kubeconform, and asserts the chart guards.
func (m *McpPlatform) HelmTemplate(ctx context.Context) (string, error) {
	helm := dag.Helm(m.Src).WithWorkDir(chartPath)
	script := `set -eu
mkdir -p /out
helm template t . > /out/default.yaml
for f in tests/values-*.yaml; do
  name=$(basename "$f" .yaml)
  case "$name" in
    values-memory-multi-replica|values-file-no-persistence|values-oidc-no-secret)
      if helm template t . -f "$f" >/dev/null 2>&1; then
        echo "GUARD BROKEN: $f must fail" >&2
        exit 1
      fi
      continue
      ;;
  esac
  helm template t . -f "$f" > "/out/$name.yaml"
done
count=$(helm template t . | grep -c 'readOnlyRootFilesystem: true' || true)
if [ "$count" -lt 2 ]; then
  echo "expected readOnlyRootFilesystem on both components, got $count" >&2
  exit 1
fi
if helm template t . | grep -A3 'kind: ConfigMap' | grep -i -e token -e secret; then
  echo "SECRET LEAK in ConfigMap" >&2
  exit 1
fi
`
	rendered, err := helm.HelmContainer().
		WithExec([]string{"sh", "-c", script}).
		Directory("/out").
		Sync(ctx)
	if err != nil {
		return "", fmt.Errorf("helm template: %w", err)
	}

	entries, err := rendered.Entries(ctx)
	if err != nil {
		return "", fmt.Errorf("list rendered manifests: %w", err)
	}
	args := []string{"/kubeconform", "-strict", "-summary"}
	for _, entry := range entries {
		args = append(args, "/manifests/"+entry)
	}
	return dag.Container().
		From("ghcr.io/yannh/kubeconform:v0.6.7").
		WithDirectory("/manifests", rendered).
		WithExec(args).
		Stdout(ctx)
}

// HelmPublish pushes the chart to an OCI registry and returns its URL.
func (m *McpPlatform) HelmPublish(ctx context.Context, registry, repository, version string, username, password *dagger.Secret) (string, error) {
	helm := dag.Helm(m.Src).
		WithWorkDir(chartPath).
		WithRepository(registry, username, password, dagger.HelmWithRepositoryOpts{IsOci: true})
	if _, err := helm.Push(registry, repository, version).Sync(ctx); err != nil {
		return "", fmt.Errorf("push helm chart: %w", err)
	}
	return helm.PushURL(ctx, registry, repository, version)
}
