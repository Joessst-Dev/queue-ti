---
name: Project toolchain versions
description: Pinned language and tool versions for queue-ti CI/CD pipelines
type: project
---

- **Go**: `1.25.5` — from `go.mod`. No separate `toolchain` directive. Use `go-version: "1.25.5"` in `actions/setup-go`.
- **Node**: No `.nvmrc` or `.node-version` file. Angular 21 + Nx 22 require **Node 22 LTS** minimum. Use `node-version: "22"` in `actions/setup-node`.
- **Nx workspace root**: `admin-ui/` — all `npx nx` commands must be run with `working-directory: admin-ui`. The `nx.json`, `project.json`, and `package-lock.json` all live there.
- **package manager**: npm (evidenced by `package-lock.json` in `admin-ui/`). Use `npm ci` for CI installs.
- **Makefile targets available**: `proto`, `deps`, `test`, `run`. No `make build` — use `go build ./cmd/server/...` directly.

**Why:** Wrong Node version causes build failures with Angular 21's native ESM requirements. Wrong working directory causes Nx to not find its workspace config.

**How to apply:** Always set `defaults.run.working-directory: admin-ui` on the frontend job. Always use Go 1.25.5 exactly for the backend job.
