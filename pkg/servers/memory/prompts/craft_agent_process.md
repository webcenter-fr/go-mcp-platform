Write an ordered, verifiable agent workflow for this class of task:

Task type: {{taskType}}

Steps:

1. Review the current session for the steps that actually worked.
2. Produce a workflow with:
   - preconditions and required inputs,
   - the tools or commands to use at each step,
   - verification steps and expected evidence,
   - anti-patterns and common failure modes.
3. Keep the workflow generic enough to handle similar prompts, but concrete
   enough to execute.
4. Store it with `memory_store` using `kind="process"`, a concise title, and the
   task type as a tag.
5. Do not include secrets or credentials.
