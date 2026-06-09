# Missionbase CLI

Standalone Missionbase command-line clients for agents and operators.

The tools are distributed as single Go binaries so they can be installed on remote agent boxes without Ruby, Bundler, or a checkout of the Rails app.

## CLIs

There are two binaries with separate auth/config scopes:

- `missionbase` — user-acting CLI for personal/user API keys.
- `missionbase-agent` — agent-acting CLI for team API keys plus an agent slug.

## Install

Install both binaries:

```bash
curl -fsSL https://raw.githubusercontent.com/Quantum-Fire-Labs/missionbase-cli/main/scripts/install.sh | bash
```

Install only one binary:

```bash
curl -fsSL https://raw.githubusercontent.com/Quantum-Fire-Labs/missionbase-cli/main/scripts/install.sh | bash -s -- missionbase-agent
```

The installer downloads the latest public GitHub release binaries for your OS/architecture and installs them to `~/.local/bin`.

## User CLI

Create a personal API key in Missionbase, then run:

```bash
missionbase auth set-token YOUR_USER_TOKEN
missionbase auth status
missionbase me
```

Credentials are stored at:

```text
~/.config/missionbase/credentials
```

## Agent CLI

Create/use a team API key that allows agent acting, then run:

```bash
missionbase-agent auth set-token YOUR_TEAM_TOKEN
missionbase-agent use test
missionbase-agent auth status
missionbase-agent me
missionbase-agent work
missionbase-agent members
```

Global agent CLI credentials are stored at:

```text
~/.config/missionbase-agent/credentials
```

The selected agent can be set per directory with:

```bash
missionbase-agent use <agent-slug>
```

That writes this file in the current directory:

```text
.missionbase-agent.json
```

`missionbase-agent` searches the current directory and parent directories for `.missionbase-agent.json`, so each project/worktree can use a different agent while sharing the same global team token.

Example `.missionbase-agent.json`:

```json
{
  "agent_slug": "test"
}
```

## Updating

Each binary updates itself:

```bash
missionbase update
missionbase-agent update
```

Useful variants:

```bash
missionbase update --check
missionbase update --force
missionbase-agent update --check
missionbase-agent update --force
```

## Current commands

```bash
missionbase version
missionbase auth status
missionbase auth set-token <token> [--base-url URL]
missionbase me
missionbase get /api/v1/users/me
missionbase update

missionbase-agent version
missionbase-agent auth status
missionbase-agent auth set-token <team-token> [--base-url URL] [--agent slug]
missionbase-agent use <agent-slug> [--base-url URL]
missionbase-agent me
missionbase-agent work
missionbase-agent tasks
missionbase-agent task feed <task-id> [--limit N]
missionbase-agent task comments <task-id> [--limit N]
missionbase-agent task participants list <task-id>
missionbase-agent task participants add <task-id> --user <user-id-or-mention>
missionbase-agent task participants add <task-id> --agent <agent-slug>
missionbase-agent conversation show <feed-id> [--limit N]
missionbase-agent members [--box ID]
missionbase-agent get /api/v1/agent/me
missionbase-agent update
```

`missionbase-agent members` lists group members, including mention handles/usernames to use when tagging humans or agents. `missionbase-agent task participants ...` adds and lists task participants through high-level commands. `get` is included as a low-level escape hatch while higher-level task/page/team commands are ported.

## Agent check helper

`scripts/missionbase-agent-check` is the local fleet check script used by timers on agent hosts. It runs `missionbase-agent work`, exits when there is no actionable work, and otherwise selects exactly one actionable item for the Pi run.

Selected direct tasks use Pi session id `missionbase-task-<task_id>`. Selected unread conversations use `missionbase-task-<task_id>` only when the conversation payload includes a task assigned to the current agent; otherwise they use `missionbase-conversation-<conversation_id>`. The script passes both `--session-id` and a descriptive `--name` to Pi.

For clean conversation scoping, the Missionbase work payload should continue to include each unread conversation's stable conversation/feed id and, for task conversations, the task id plus assignees.

## Release flow

Tag a release and push it:

```bash
git tag v0.1.3
git push origin v0.1.3
```

GitHub Actions builds and attaches platform binaries named like:

```text
missionbase-linux-amd64
missionbase-linux-arm64
missionbase-agent-linux-amd64
missionbase-agent-linux-arm64
```
