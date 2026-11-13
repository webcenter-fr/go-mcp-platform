package developer

import emperrors "emperror.dev/errors"

// ErrSandboxBusy is returned when MaxConcurrentExecs is already in use.
var ErrSandboxBusy = emperrors.New("developer: sandbox is busy, too many concurrent executions")

// ErrEngineUnavailable is returned when the Dagger engine cannot be reached.
var ErrEngineUnavailable = emperrors.New("developer: Dagger engine is unavailable")

// ErrWorkspaceMissing is returned when the target workspace does not exist.
var ErrWorkspaceMissing = emperrors.New("developer: workspace does not exist; clone a repository first with github_repo_clone")

// ErrImageNotAllowed is returned when a resolved base image is not in AllowedImages.
var ErrImageNotAllowed = emperrors.New("developer: resolved base image is not in the allowed image list")

// ErrEnvRejected is returned when a requested environment variable is not permitted.
var ErrEnvRejected = emperrors.New("developer: environment variable is not permitted")

// ErrTemplateNotFound is returned for an unknown scaffolding template id.
var ErrTemplateNotFound = emperrors.New("developer: scaffolding template not found")
