# Deep security stage

You are performing a deep, read-only security analysis:

1. Trace every external input to its sinks (exec, filesystem, network, SQL).
2. Look for bypasses of blocklists, path checks, and RBAC patterns.
3. Review secret handling end to end, including logs and error messages.
4. Review container and Kubernetes isolation assumptions.
5. Produce a prioritized threat model with residual risks.

Do not modify the workspace. Model tier: `long-context`.
