package main

import (
	"context"
	"fmt"

	"dagger/mcp-platform/internal/dagger"
)

// ImageMcp builds the MCP server image.
func (m *McpPlatform) ImageMcp() *dagger.Container {
	return dag.Image().Build(m.Src, dagger.ImageBuildOpts{Dockerfile: "Dockerfile.mcp"}).GetContainer()
}

// ImageApproval builds the approval server image.
func (m *McpPlatform) ImageApproval() *dagger.Container {
	return dag.Image().Build(m.Src, dagger.ImageBuildOpts{Dockerfile: "Dockerfile.approval"}).GetContainer()
}

// ImageDeveloper builds the developer MCP server image (includes the Dagger CLI).
func (m *McpPlatform) ImageDeveloper() *dagger.Container {
	return dag.Image().Build(m.Src, dagger.ImageBuildOpts{Dockerfile: "Dockerfile.developer"}).GetContainer()
}

// PublishImages builds and pushes both images, returning their pinned refs.
func (m *McpPlatform) PublishImages(ctx context.Context, registry, repository, version string, username, password *dagger.Secret) ([]string, error) {
	images := []struct {
		name       string
		dockerfile string
	}{
		{name: "mcp", dockerfile: "Dockerfile.mcp"},
		{name: "approval", dockerfile: "Dockerfile.approval"},
		{name: "developer", dockerfile: "Dockerfile.developer"},
	}
	refs := make([]string, 0, len(images))
	for _, image := range images {
		build := dag.Image().Build(m.Src, dagger.ImageBuildOpts{Dockerfile: image.dockerfile})
		digest, err := build.Push(ctx, repository+"/"+image.name, version, registry, dagger.ImageBuildPushOpts{
			WithRegistryUsername: username,
			WithRegistryPassword: password,
		})
		if err != nil {
			return nil, fmt.Errorf("push %s image: %w", image.name, err)
		}
		refs = append(refs, fmt.Sprintf("%s/%s/%s:%s@%s", registry, repository, image.name, version, digest))
	}
	return refs, nil
}
