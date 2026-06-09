# Missionbase CLI

Standalone Missionbase command-line client for agents and operators.

The CLI is intentionally distributed as a single Go binary so it can be installed on remote agent boxes without Ruby, Bundler, or a checkout of the Rails app.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/Quantum-Fire-Labs/missionbase-cli/main/scripts/install.sh | bash
```

The installer downloads the latest GitHub release binary for your OS/architecture and installs it to `~/.local/bin/missionbase`.

For private repositories, provide a token that can read releases:

```bash
export GITHUB_TOKEN=ghp_...
curl -fsSL -H "Authorization: Bearer $GITHUB_TOKEN" \
  https://raw.githubusercontent.com/Quantum-Fire-Labs/missionbase-cli/main/scripts/install.sh | bash
```

## Auth

Create a personal API key in Missionbase, then run:

```bash
missionbase auth set-token YOUR_TOKEN
missionbase auth status
```

Credentials are stored at:

```text
~/.config/missionbase/credentials
```

Use a different Missionbase instance with:

```bash
missionbase auth set-token YOUR_TOKEN --base-url https://dash.missionbase.app
```

## Updating

```bash
missionbase update
```

For private repositories, `missionbase update` also honors `GITHUB_TOKEN`.

Useful variants:

```bash
missionbase update --check
missionbase update --force
```

## Current commands

```bash
missionbase version
missionbase auth status
missionbase auth set-token <token> [--base-url URL]
missionbase me
missionbase get /api/v1/users/me
missionbase update
```

`missionbase get` is included as a low-level escape hatch while the higher-level task/page/team commands are ported from the previous CLI.

## Release flow

Tag a release and push it:

```bash
git tag v0.1.0
git push origin v0.1.0
```

GitHub Actions builds and attaches platform binaries named like:

```text
missionbase-linux-amd64
missionbase-linux-arm64
missionbase-darwin-amd64
missionbase-darwin-arm64
```
