// A Dagger module for the go-mcp-platform repository: Go gates, container
// images, Helm chart validation and the CI orchestrator.
package main

import (
	"dagger/mcp-platform/internal/dagger"
)

// McpPlatform is the repository CI module.
type McpPlatform struct {
	Src *dagger.Directory
}

// New creates the module from the repository source.
func New(
	// +defaultPath="/"
	// +ignore=["bin", ".git", "**/node_modules", "*.test", "ci-out", "go.work", "go.work.sum"]
	src *dagger.Directory,
) *McpPlatform {
	return &McpPlatform{Src: src}
}
