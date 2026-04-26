---
name: Run lint before finishing a frontend feature
description: Always run npx nx lint after completing Angular frontend work, before reporting a feature as done
type: feedback
---

Always run `npx nx lint` (from `admin-ui/`) as the final step of any Angular frontend feature, after tests pass. Do not report a feature as complete without a clean lint run.

**Why:** Linting issues were found after the Prometheus metrics / stats chart feature was shipped — empty mock methods that violated `@typescript-eslint/no-empty-function`.

**How to apply:** After `npx nx test` passes, run `npx nx lint`. Fix any errors before summarising the work as done. This applies to agent-delegated frontend work too — verify lint in the parent context after the agent returns.
