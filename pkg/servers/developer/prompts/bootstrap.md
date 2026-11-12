# Bootstrap the agentic developer experience

Bootstrap this project for an AI coding agent:

1. Call `developer_scaffold_list` and select the templates for the project's
   language.
2. Call `developer_scaffold_get` for each selected template and review it.
3. Call `developer_scaffold_apply` with the selected ids, the repository owner
   and name, and `dryRun=true` first.
4. Re-run with `dryRun=false` once the plan is correct.
5. Commit the generated `AGENTS.md`, lifecycle spec, and opencode settings.

Never overwrite an existing file without reviewing it first.
