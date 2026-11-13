Craft a reusable prompt template for the following goal:

Goal: {{goal}}

Steps:

1. Review the current session for clarifications, constraints, and examples that
   made the work succeed.
2. Write a self-contained prompt template that another agent can reuse without
   this conversation. Include the role, the task, the inputs, the constraints,
   and the expected output format.
3. Remove session-specific details that would not generalize.
4. Store the template with `memory_store` using `kind="prompt"`, a concise
   title, and the goal as a tag.
5. Do not include secrets or credentials.
