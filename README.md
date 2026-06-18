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
missionbase work
missionbase teams
missionbase boxes
missionbase tasks assigned
```

The user CLI is intentionally user-acting only: it reads `~/.config/missionbase/credentials`, does not read `.missionbase-agent.json`, and never sends `X-Missionbase-Agent-Slug`.

The user CLI covers day-to-day user workflows with high-level commands for `missionbase work`, teams, users, boxes, tasks, task feeds, conversations, standalone box discussions, notes search, and document create/show/update. Safe user-acting writes cover task create/update/status/complete/comment, conversation comments, and box discussion creation. It also includes raw `get` plus optional raw `post`/`patch`/`delete` helpers for uncommon API paths; raw write helpers act as your signed-in user and should not replace high-level commands for common workflows.

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
missionbase-agent work [--next|--next-task]
missionbase-agent listen --once
missionbase-agent dm list
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
missionbase work
missionbase teams
missionbase users lookup <query-or-mention> [--team <team-id>]
missionbase team show <team-id>
missionbase team members <team-id>
missionbase boxes [--team <team-id>]
missionbase box show <box-id>
missionbase boxes tasks <box-id> [--status STATUS] [--status-category open|done|canceled] [--task-status-ids IDS] [--page N] [--per-page N]
missionbase boxes discussions <box-id> [--page N] [--per-page N]
missionbase boxes discussions create <box-id> --title TITLE --body TEXT
missionbase boxes documents create <box-id> --title TITLE --body TEXT
missionbase boxes task-statuses <box-id>
missionbase boxes statuses <box-id>
missionbase notes search <query> [--team <team-id>]
missionbase document show <document-id> [--format markdown|html|plain-text]
missionbase document update <document-id> [--title TITLE] --body TEXT
missionbase tasks assigned [--page N] [--per-page N]
missionbase tasks visible [--page N] [--per-page N]
missionbase task create --title TITLE [--box ID] [--description TEXT] [--deadline YYYY-MM-DD] [--status STATUS] [--task-status-id ID] [--assign-user ID] [--attach PATH] [--attach-blob SIGNED_ID_OR_SGID]
missionbase task update <task-id> [--title TITLE] [--description TEXT] [--box ID] [--status STATUS] [--task-status-id ID]
missionbase task status <task-id> <status>
missionbase task complete <task-id>
missionbase task comment <task-id> --body TEXT [--attach PATH] [--attach-blob SIGNED_ID_OR_SGID]
missionbase task assign <task-id> --user <user-id-or-mention> [--team <team-id>]
missionbase task unassign <task-id> --user <user-id-or-mention> [--team <team-id>]
missionbase task participants list <task-id>
missionbase task participants add <task-id> --user <user-id-or-mention> [--team <team-id>]
missionbase task show <task-id>
missionbase task feed <task-id> [--limit N]
missionbase task comments <task-id> [--limit N]
missionbase conversations [--page N] [--per-page N]
missionbase conversation show <feed-id> [--limit N]
missionbase conversation comment <feed-id> --body TEXT [--attach PATH] [--attach-blob SIGNED_ID_OR_SGID]
missionbase get /api/v1/users/me
missionbase post /api/path --json JSON
missionbase patch /api/path --json JSON
missionbase delete /api/path
missionbase update

missionbase-agent version
missionbase-agent auth status
missionbase-agent auth set-token <team-token> [--base-url URL] [--agent slug]
missionbase-agent use <agent-slug> [--base-url URL]
missionbase-agent me
missionbase-agent work [--next|--next-task]
missionbase-agent listen [--timeout N] [--offset ID] [--once]
missionbase-agent dm list [--limit N]
missionbase-agent dm show <chat-id>
missionbase-agent dm send --to <handle> --body-file /tmp/body.md
missionbase-agent dm send --chat <chat-id> --body-file /tmp/body.md
missionbase-agent agent create --name "Fleet Worker" --slug fleet-worker [--description "Handles fleet tasks"]
missionbase-agent agent archive fleet-worker --yes
missionbase-agent agent restore fleet-worker --yes
missionbase-agent agent boxes add fleet-worker --box <box-id> [--box <box-id>]
missionbase-agent document fetch <document-id> [--format markdown|html|plain-text]
missionbase-agent document create --box <box-id> --title "Doc title" --body-file /tmp/document.md
missionbase-agent document edit <document-id> [--title "New title"] --body-file /tmp/document.md
missionbase-agent tasks
missionbase-agent tasks --user <user-id-or-mention> [--scheduled actionable|future|all]
missionbase-agent task create --title "Task title" --box <box-id> [--deadline YYYY-MM-DD] [--scheduled-at DATETIME] [--assign-agent <agent-slug> | --assign-user <user-id-or-mention>] [--description-file /tmp/description.md] [--participant-user <user-id-or-mention>] [--attach /path/to/image.png] [--attach-blob <signed-id-or-sgid>]
missionbase-agent task update <task-id> --deadline YYYY-MM-DD
missionbase-agent task update <task-id> --no-deadline
missionbase-agent task update <task-id> --scheduled-at DATETIME
missionbase-agent task update <task-id> --no-scheduled-at
missionbase-agent task assign <task-id> --user <user-id-or-mention>
missionbase-agent task assign <task-id> --agent <agent-slug>
missionbase-agent task unassign <task-id> --user <user-id-or-mention>
missionbase-agent task unassign <task-id> --agent <agent-slug>
missionbase-agent task unassign <task-id> --self
missionbase-agent task comment <task-id> --body-file /tmp/body.md [--attach /path/to/image.png]
missionbase-agent task status <task-id> <status>
missionbase-agent task move <task-id> --box <box-id>
missionbase-agent task complete <task-id>
missionbase-agent task feed <task-id> [--limit N]
missionbase-agent task comments <task-id> [--limit N]
missionbase-agent task participants list <task-id>
missionbase-agent task participants add <task-id> --user <user-id-or-mention>
missionbase-agent task participants add <task-id> --agent <agent-slug>
missionbase-agent conversation show <feed-id> [--limit N]
missionbase-agent conversation comment <feed-id> --body-file /tmp/body.md [--attach /path/to/image.png]
missionbase-agent members [--box ID]
missionbase-agent boxes tasks <box-id> [--status STATUS | --status-category open|done|canceled | --task-status-ids IDS] [--scheduled actionable|future|all] [--page N] [--per-page N]
missionbase-agent boxes discussions <box-id> [--page N] [--per-page N]
missionbase-agent boxes discussions create <box-id> --title TITLE --body-file /tmp/body.md
missionbase-agent boxes files <box-id> [--query QUERY] [--page N] [--per-page N]
missionbase-agent boxes task-statuses <box-id>
missionbase-agent boxes statuses <box-id>
missionbase-agent get /api/v1/agent/me
missionbase-agent update
```

User CLI read and write commands return raw JSON directly from the Missionbase API. `missionbase me` calls `/api/v1/users/me` only, `missionbase work` calls `/api/v1/users/work` to return the current user, assigned/open tasks, unread conversations, and metadata, user/mention lookup uses `/api/v1/users/lookup` and `/api/v1/teams/:team_id/members`, and assignment/participant writes use user-acting task endpoints. The user CLI never calls agent-only member endpoints, never supports `--agent` assignment/participants, and agent identity, agent-management, and DM workflows remain exclusive to `missionbase-agent`.

User CLI write commands use JSON requests when no attachments are present and multipart requests when repeated `--attach PATH` or `--attach-blob SIGNED_ID_OR_SGID` flags are used. Local attachments are limited to PNG, JPEG, GIF, and WEBP images up to 5 MB. User-authored task descriptions, comments, conversation comments, and box discussion bodies are Markdown-capable and normalize accidental escaped newline sequences outside quoted/code contexts. Comment body aliases `--body`, `--comment`, `--message`, and `--text` are supported.

`missionbase users lookup <query>` calls user lookup directly. `missionbase users lookup @mention --team <team-id>` resolves a team member mention. Task assignment and participant commands accept numeric user ids directly; when resolving `@mention`, pass `--team` or let the CLI derive the team from the task when the task response includes box/team context. If team context cannot be inferred, the CLI asks for `--team` or a numeric user id.

`missionbase-agent boxes discussions ...` lists standalone box discussions only; it does not include task conversations. `missionbase-agent boxes discussions create ...` creates a standalone box discussion/post and prints the created discussion JSON. `missionbase-agent boxes files ...` lists/searches files in an accessible box and currently returns box documents with identifiers, canonical URLs, fetch metadata, creator/owner, timestamps, status, and pagination metadata. `missionbase-agent document fetch ...` prints a document body and reports the document URL when the API response includes one; `--format` accepts `markdown` (default), `html`, or `plain-text`. `missionbase-agent document create ...` creates a box document and prints the created document JSON, including its URL. `missionbase-agent document edit ...` updates an existing document by creating a new document version. `missionbase-agent task comment ...` posts a comment/reply to the task conversation feed. `missionbase-agent conversation comment ...` posts a reply to any readable feed conversation, including task conversations and standalone discussion feeds.

Task comment, conversation comment, box discussion create, document, and DM bodies are Markdown-capable by default; Missionbase renders headings, bold/italic, inline code, fenced code blocks, bullet/numbered lists, blockquotes, and links as sanitized rich text while ordinary plain text continues to display normally. These agent-authored body fields also defensively normalize accidental escaped newline sequences (`\\n`, `\\r`, and `\\r\\n`) into real line breaks outside quoted/backticked code contexts.

Agent-authored posting bodies are file-only: use `--body-file PATH` for DM bodies, task comments, conversation comments, box discussion bodies, and document bodies. Task creation descriptions use `--description-file PATH`. Inline body/description flags and stdin body input (`--body-stdin` or `--body-file -`) are intentionally unsupported for these write flows so Markdown, backticks, and shell-sensitive content are read from disk instead of passing through fragile shell quoting or piped interactive flows.

Recommended workflow:

```bash
cat > /tmp/missionbase-comment.md <<'EOF'
## Summary

- Preserved inline code like `context: "modal"`.
- Preserved fenced code blocks.

~~~text
literal `backticks` and "quotes"
~~~
EOF

missionbase-agent task comment 123 --body-file /tmp/missionbase-comment.md
```

When file content accidentally contains escaped newline sequences (`\n`, `\r`, or `\r\n`), the CLI continues to normalize them to real line breaks outside quoted/backticked code contexts. Quoted JSON, shell snippets, and inline-code literals such as `printf 'a\\nb'` are preserved.

`missionbase-agent task create --scheduled-at DATETIME` and `missionbase-agent task update <task-id> --scheduled-at DATETIME` set `scheduled_at` separately from `deadline`; use `--no-scheduled-at` (or `--clear-scheduled-at`) to clear scheduling without changing the deadline. The API parses schedule datetimes in the acting user's timezone when no offset is included, so include an ISO-8601 offset or `Z` for an absolute instant. Normal agent work/task endpoints keep the API default scheduled filter, hiding future scheduled tasks until actionable; use `--scheduled future` or `--scheduled all` on supported task listings only when explicitly discovering scheduled tasks.

`missionbase task assign ...` and `missionbase task unassign ...` manage user assignments for existing tasks with the user-acting CLI. Use `--user` with a numeric user id or `@mention`; mention resolution uses `--team <team-id>` or task-derived team context when available. `missionbase task participants list ...` lists task participants, and `missionbase task participants add ... --user ...` adds user participants only.

`missionbase-agent task assign ...` and `missionbase-agent task unassign ...` manage assignments for existing tasks using the Missionbase assignment API. Use `--user` with a numeric user id or `@mention`, `--agent` with an agent slug, or `task unassign <task-id> --self` to safely remove the currently selected agent from a task after handing it off.

`missionbase-agent task move <task-id> --box <box-id>` moves an existing task to another agent-accessible box through the task update API. Missionbase preserves the task record, comments/feed, attachments, participants, and assignments; the server remaps the task status to a safe status in the destination box when workflows differ.

`missionbase-agent boxes task-statuses <box-id>` (alias: `boxes statuses`) prints all configured task statuses for an agent-accessible box as JSON, including custom and archived statuses. Each status includes `id`, `key`, `name`, `category`, `position`, `color`, `default_open`, `primary_done`, `primary_canceled`, and `archived`.

Task create/comment and conversation comment accept repeated `--attach PATH` flags for local image files and repeated `--attach-blob SIGNED_ID_OR_SGID` flags to reuse an existing Missionbase ActiveStorage blob from an attachment response. Supported local/blob attachment types are PNG, JPEG, GIF, WEBP, HEIC, and HEIF images up to 5 MB each. Attachments are appended inline to the task description or comment rich text so they are visible in the Missionbase UI.

Examples:

```bash
missionbase-agent task create --box 2 --title "Investigate screenshot" --description-file /tmp/description.md --attach /tmp/screenshot.png
missionbase users lookup Daniel --team 2
missionbase task assign 123 --user @DanielLemky --team 2
missionbase task participants add 123 --user 42
missionbase-agent task create --box 2 --assign-agent missionbase-dev --title "Assigned investigation"
missionbase-agent task assign 123 --user @DanielLemky
missionbase-agent task unassign 123 --self
missionbase-agent task move 123 --box 45
missionbase-agent task comment 123 --body-file /tmp/comment.md --attach /tmp/repro.webp
missionbase-agent boxes discussions 2
missionbase-agent boxes discussions create 2 --title "Release workflow planning" --body-file /tmp/proposal.md
missionbase-agent boxes files 2
missionbase-agent boxes files 2 --query runbook --per-page 25
missionbase-agent document fetch 789
missionbase-agent document fetch 789 --format html
missionbase-agent document fetch 789 --format plain-text
missionbase-agent document create --box 2 --title "Runbook" --body-file /tmp/runbook.md
missionbase-agent document edit 789 --title "Updated runbook" --body-file /tmp/runbook-v2.md
missionbase-agent conversation comment 456 --body-file /tmp/reply.md --attach /tmp/context.png
missionbase-agent task comment 123 --body-file /tmp/findings.md
missionbase-agent task comment 123 --body-file /tmp/comment.md --attach-blob "<signed-id-or-sgid>"
```

### Agent management

`missionbase-agent agent create ...` creates a new agent on the authenticated team and prints the created agent JSON, including its id and slug. It requires a team API key with `agents:create` permission; invalid or duplicate slugs are returned as API validation errors.

`missionbase-agent agent boxes add ...` adds an agent to one or more boxes and prints JSON with the agent and membership status (`created` or `existing`) for each box. It requires `agents:update` and `boxes:update` permissions.

`missionbase-agent agent archive ... --yes` is the supported safe delete flow for agents. It archives/deactivates the agent instead of hard-deleting it, preserving historical task/comment/message attribution. Archived agents are removed from active assignment, mention, DM, and box membership choices; agent-owned API keys are revoked; and selected-agent credentials using the archived slug are rejected. The server refuses to archive an agent that is still assigned to open tasks according to each box's configured task-status categories, so hand off or close that work first.

`missionbase-agent agent restore ... --yes` restores/reactivates an archived agent with its existing identity and box memberships so it can be assigned and used for new work again. Restoring does not recreate agent-owned API keys that were revoked during archival; create new credentials if the restored agent needs to authenticate.

```bash
missionbase-agent agent create --name "Fleet Worker" --slug fleet-worker --description "Handles fleet tasks"
missionbase-agent agent boxes add fleet-worker --box 2
missionbase-agent agent boxes add 42 --box 2 --box 7
missionbase-agent agent archive fleet-worker --yes
missionbase-agent agent restore fleet-worker --yes
```

These management commands use the authenticated team token and do not require a selected agent slug, so they can be used during initial fleet bootstrap, cleanup, and restore.

### Agent long polling

`missionbase-agent listen` long-polls `/api/v1/agent/updates` for actionable agent events. This is Telegram-style polling: the request blocks until an update is available or the timeout expires, then the CLI immediately requests the next offset.

```bash
missionbase-agent listen
missionbase-agent listen --timeout 30
missionbase-agent listen --offset 123
missionbase-agent listen --once
```

The update stream is intended for events that should wake an agent up:

- `task_assigned` — a task was assigned to the current agent.
- `conversation_unread` — a task or discussion conversation became unread for the current agent, usually through a mention or participant update.
- `direct_message` — another agent sent this agent a direct message.

`listen` prints each JSON response. Use `--once` for scripts that want one long-poll cycle and then exit.

### Agent direct messages

`missionbase-agent dm ...` sends and reads direct messages with users or agents on the same team. The sender is always the currently selected agent from `missionbase-agent use <agent-slug>`; `--to` identifies the recipient by their handle/username/slug.

```bash
missionbase-agent dm send --to codex --body-file /tmp/dm.md
missionbase-agent dm list
missionbase-agent dm list --limit 10
missionbase-agent dm show 42
missionbase-agent dm send --chat 42 --body-file /tmp/reply.md
```

`dm send --to <handle>` creates or reuses a unified Missionbase chat with that recipient. Messages to agents create a `direct_message` update for each recipient agent, so a recipient running `missionbase-agent listen` receives it without periodic `work` polling. Received message payloads include each sender's `handle`, so replies can use the same `--to` form. Human-to-agent and agent-to-agent DMs use the same chat/message backend. DM `--body-file` values support Markdown by default and are sanitized before rendering in Missionbase.

### Rich text and attachments

`missionbase-agent` prints the Missionbase API JSON response as-is for read commands. Task descriptions, task feed comments, unread work items, and DM messages include backwards-compatible plain text fields plus rich text fields when the server provides them:

- `description`, `body`, or `content`: existing plain text or HTML-compatible field, depending on the command.
- `description_html`, `body_html`, or `content_html`: rendered rich-text HTML.
- `description_rich_text`, `body_rich_text`, or `content_rich_text`: object with `plain_text`, `html`, and `attachments`.
- `attachments`: convenience copy of the rich-text attachment list. File/image attachments include `filename`, `content_type`, `byte_size`, `image`, and a relative download `url` where supported.

Use `missionbase-agent tasks`, `missionbase-agent work`, `missionbase-agent task feed <task-id>`, `missionbase-agent task comments <task-id>`, and `missionbase-agent dm show <chat-id>` to inspect this rich content. Agents should use the plain text fields for prompt context and consult the `attachments` arrays to discover files/images that may need separate handling.

### Other agent commands

`missionbase-agent members` lists group members, including mention handles/usernames to use when tagging humans or agents. `missionbase-agent task status <task-id> <status>` updates a task status and relies on the server to validate the task's box-specific statuses; `task move <task-id> --box <box-id>` moves a task to another accessible box and relies on the server to remap the task status safely for the destination box; `complete` is routed through Missionbase's complete endpoint so completion metadata and recurring follow-ups are handled correctly. `missionbase-agent task participants ...` adds and lists task participants through high-level commands. `missionbase-agent boxes tasks <box-id>` lists open-category tasks in an accessible box by default; use `--status-category`, `--task-status-ids`, legacy `--status`, `--page`, and `--per-page` to refine results. `missionbase-agent boxes discussions <box-id>` lists standalone box discussions only, while `conversation show/comment` remains the generic feed-conversation surface for both task conversations and discussion feeds. `missionbase-agent boxes files <box-id>` lists and searches box files/documents as JSON; use `--query`, `--page`, and `--per-page` to filter or paginate. Box/task API responses include `task_statuses`/`task_status_id`, `status_label`, `status_category`, `task_status_position`, and `status_color` so clients can discover and display allowed custom statuses. `get` is included as a low-level escape hatch while higher-level task/page/team commands are ported.

## Agent check helper

`scripts/missionbase-agent-check` is the local fleet check script used by timers on agent hosts. It currently runs `missionbase-agent work`, exits when there is no actionable work, and otherwise selects exactly one actionable item for the Pi run. `missionbase-agent work --next`/`--next-task` asks the server for only the next assigned open task, selected by box id and task box position ordering with stable tie-breakers, and omits unread conversations/direct messages. Newer agent hosts can use `missionbase-agent listen --once` before invoking this check to reduce periodic polling latency.

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
