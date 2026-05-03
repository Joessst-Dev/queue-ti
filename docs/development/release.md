# Release Management

queue-ti uses [Calendar Versioning](https://calver.org) (CalVer) in the format `vYYYY.MM.PATCH`.

## Versioning

A single version tag on `main` drives all release artifacts:

| Artifact | Published as |
|---|---|
| Docker image | Exact tag (e.g. `ghcr.io/joessst-dev/queue-ti:v2026.05.0`); rolling `preview` tag for preview releases; `latest` tag for stable releases |
| Go client library | `github.com/Joessst-Dev/queue-ti/clients/go-client@vYYYY.MM.PATCH` (separate sub-module tag `clients/go-client/vYYYY.MM.PATCH`) |
| GitHub Release | Auto-generated changelog from merged PR titles |

### Release Types

- **Stable release** — Tag: `vYYYY.MM.PATCH` (e.g. `v2026.05.0`) — Published as `:vYYYY.MM.PATCH` and `:latest` on GHCR
- **Preview release** — Tag: `vYYYY.MM.PATCH-preview.N` (e.g. `v2026.05.0-preview.1`) — Published as `:vYYYY.MM.PATCH-preview.N` and `:preview` rolling pointer on GHCR
- **Release candidate** — Tag: `vYYYY.MM.PATCH-rc.N` (e.g. `v2026.05.0-rc.1`) — Published as `:vYYYY.MM.PATCH-rc.N` (no `:latest` tag)

## Cutting a Release

1. **Ensure `main` is in a releasable state** — CI must be green, all features merged, tests passing.

2. **Push a version tag** in CalVer format:
   ```bash
   git tag v2026.05.0           # Stable release
   git tag v2026.05.0-preview.1 # Preview release
   git tag v2026.05.1-rc.1      # Release candidate
   git push origin <tag>
   ```

3. **The release pipeline runs automatically** (only on tags matching `v[0-9][0-9][0-9][0-9].[0-9][0-9].[0-9]*`):
   - Runs the full backend and frontend test suites (release is blocked on test failure)
   - Builds and pushes a multi-arch Docker image (`linux/amd64` + `linux/arm64`) to GHCR
     - Always pushes the exact tag (e.g. `:v2026.05.0-preview.1`)
     - For preview releases (tag contains `-preview`), also pushes `:preview` rolling pointer
     - For stable releases (no `-preview` or `-rc`), also pushes `:latest` rolling pointer
   - Creates a Go sub-module tag `clients/go-client/vYYYY.MM.PATCH` so the client library is consumable as `go get github.com/Joessst-Dev/queue-ti/clients/go-client@vYYYY.MM.PATCH`
   - Publishes a GitHub Release with auto-generated notes from merged PRs

4. **Monitor the run** at **Actions → Release** in the GitHub repository.

## Using a Release

### Docker

Pull a specific release:

```bash
# Stable release
docker pull ghcr.io/joessst-dev/queue-ti:v2026.05.0

# Preview release (rolling pointer)
docker pull ghcr.io/joessst-dev/queue-ti:preview

# Latest stable release (rolling pointer)
docker pull ghcr.io/joessst-dev/queue-ti:latest
```

Or pin the image tag in `docker-compose.yaml`:

```yaml
services:
  queueti:
    image: ghcr.io/joessst-dev/queue-ti:v2026.05.0
    # OR: ghcr.io/joessst-dev/queue-ti:latest (always pulls the latest stable)
```

### Go Client Library

```bash
# Pin to a specific version
go get github.com/Joessst-Dev/queue-ti/clients/go-client@v2026.05.0

# Or latest
go get github.com/Joessst-Dev/queue-ti/clients/go-client@latest
```

The client library is published as a Go sub-module, so it can be imported and used independently:

```go
import client "github.com/Joessst-Dev/queue-ti/clients/go-client"

c, _ := client.Dial("localhost:50051", client.WithInsecure())
defer c.Close()
```

## CI/CD Pipelines

### Continuous Integration (`.github/workflows/ci.yml`)

Runs on every push and pull request:

| Job | What it does |
|---|---|
| `backend` | `go build`, Ginkgo test suite with a real PostgreSQL container |
| `frontend` | Angular production build, Vitest unit tests, ESLint |
| `build-image` | Docker image build (no push) — catches Dockerfile regressions early |

### Release Pipeline (`.github/workflows/release.yml`)

Runs only on version tags matching `v[0-9][0-9][0-9][0-9].[0-9][0-9].[0-9]*`:

| Job | What it does |
|---|---|
| `backend` | Same as CI: build and test (gates the release) |
| `frontend` | Same as CI: build, test, and lint (gates the release) |
| `publish-image` | Builds multi-arch Docker image (linux/amd64 + linux/arm64) and pushes to GHCR with appropriate tags (exact, `:preview`, and/or `:latest`) |
| `create-release` | Creates GitHub Release with auto-generated notes and tags the client Go sub-module (`clients/go-client/vYYYY.MM.PATCH`) |

## Changelog

Release notes are generated automatically by GitHub from merged PR titles and commit messages since the previous tag. To produce meaningful changelogs:

- Use **descriptive PR titles** that clearly state what changed (e.g., "feat: add consumer groups", not "Update code")
- **Squash-merge** feature branches to keep the commit history clean
- Use conventional commit format: `feat:`, `fix:`, `docs:`, `chore:`, etc.

GitHub will automatically group changes by type and generate the release notes.

## Version Bumping Strategy

### Month/Year Changes

When the month or year changes (e.g., from April to May, or from 2025 to 2026):

```bash
git tag v2026.05.0  # First release of May 2026
```

### Patch Releases

For multiple releases in the same month, increment the patch version:

```bash
git tag v2026.05.0    # First May release
git tag v2026.05.1    # Second May release
git tag v2026.05.2    # Third May release
```

### Preview and RC Releases

For testing releases before a stable version:

```bash
git tag v2026.05.0-preview.1   # First preview
git tag v2026.05.0-preview.2   # Second preview
git tag v2026.05.0-rc.1        # Release candidate
git tag v2026.05.0             # Stable release
```

## Hotfix Releases

For critical fixes after a release:

1. Create a hotfix branch from the stable tag:
   ```bash
   git checkout -b hotfix/fix-description v2026.05.0
   ```

2. Make your fix and test thoroughly:
   ```bash
   git commit -m "fix: critical issue in dequeue"
   ```

3. Create a PR and merge to `main`:
   ```bash
   git push origin hotfix/fix-description
   # Create PR, merge when CI passes
   ```

4. Tag and release from `main`:
   ```bash
   git tag v2026.05.1
   git push origin v2026.05.1
   ```

## Deprecation and Breaking Changes

### Policy

- **Deprecations** are announced in release notes but do not block users
- **Breaking changes** are only made in major version bumps (across calendar years)
- Features are marked deprecated for at least one release before removal

### Communication

- Document deprecations in the README and API docs
- Add deprecation warnings to logs and error messages
- Announce in GitHub Releases with clear migration guidance

Example:

```
## v2026.05.0 - May 2026

### Deprecations
- `WithBearerToken()` in Go client is deprecated; use `WithAuth()` instead
- HTTP Basic Auth will be removed in v2027.01.0; migrate to JWT tokens

### Migration Guide
See https://docs.queue-ti.dev/migration-v2026.05
```
