Review the current conversation and distill it into durable long-term memory.

Steps:

1. Identify the problem, the resolution, and any reusable knowledge.
2. Store a `discussion` memory with `memory_store` containing a concise summary
   that can directly answer the same question in a future session.
3. Record any durable `preference`, `best_practice`, `process`, or `prompt`
   entries discovered during the session with `memory_store`.
4. Use a stable `scope` (for example `repo:org/name`, `lang:golang`, or
   `cluster:prod`) and a few searchable tags.
5. Do not store secrets, credentials, or transient task status.

Prefer several small, focused memories over one large dump. Each memory must be
self-contained.
