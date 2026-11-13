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
curl -fsSL https://raw.githubusercontent.com/Quantum-Fire-Labs/missionbase-cli/master/scripts/install.sh | bash
```

Install only one binary:

```bash
curl -fsSL https://raw.githubusercontent.com/Quantum-Fire-Labs/missionbase-cli/master/scripts/install.sh | bash -s -- missionbase-agent
```

The installer downloads the latest public GitHub release binaries for your OS/architecture and installs them to `~/.local/bin`.

## User CLI

Create a personal API key in Missionbase, then run:

```bash
missionbase auth set-token YOUR_USER_TOKEN
missionbase auth status
missionbase me
missionbase work
missionbase scratchpad show
missionbase scratchpad update --body TEXT
missionbase scratchpad edit --body-file PATH
missionbase teams
missionbase projects
missionbase tasks assigned
```

The user CLI is intentionally user-acting only: it reads `~/.config/missionbase/credentials`, does not read `.missionbase-agent.json`, and never sends `X-Missionbase-Agent-Slug`.

The user CLI covers day-to-day user workflows with high-level commands for `missionbase work`, scratchpad show/update, teams, users, boxes, tasks, task discussion messages, conversations, standalone box discussions, notes search, and document create/show/update. Safe user-acting writes cover task create/update/status/complete/message, discussion/document/file messages, and box discussion creation. It also includes raw `get` plus optional raw `post`/`patch`/`delete` helpers for uncommon API paths; raw write helpers act as your signed-in user and should not replace high-level commands for common workflows.

Credentials are stored at:

```text
~/.config/missionbase/credentials
```

### Host-side Pi sessions

Configure the SSH destination for the Missionbase runner host once, then open
the real host-side Pi TUI from the desktop:

```bash
missionbase pi configure --host daniel@agents-host
missionbase pi
missionbase pi --team Quantum Fire Labs
missionbase pi --agent fleet-architect --task 123
missionbase pi --agent missionbase-dev --discussion 456 --one-shot
```

The command keeps one SSH/PTY connection open for the attached session and runs
`sudo -n /opt/missionbase-runner/bin/mb-pi` remotely. The runner host retains
agent credentials, repositories, tools, workspaces, and Pi JSONL sessions. The
SSH user must have a narrowly scoped passwordless sudo rule for that exact
launcher; do not grant a general-purpose passwordless root shell.

## Agent CLI

Create/use a team API key that allows agent acting, then run:

```bash
missionbase-agent auth set-token YOUR_TEAM_TOKEN
missionbase-agent use test
missionbase-agent auth status
missionbase-agent me
missionbase-agent work [--next|--next-task]
missionbase-agent scratchpad show --user USER
missionbase-agent scratchpad edit --user USER --body-file PATH
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

### Projects (formerly boxes)

Boxes are now called projects. Both CLIs use the new names, and the old ones keep working until every machine has upgraded:

- `projects ...` runs the same subcommands as `boxes ...` (`missionbase-agent projects tasks 2`, `missionbase projects files 2`), `missionbase project show` is `box show`, and `missionbase-agent agent projects add` is `agent boxes add`.
- `--project` (and `--project-id`) is the flag; `--box` and `--box-id` are its old names. `task move <task-id> --project <project-id>` also takes `--box`.
- Requests go to `/api/v1/projects/...` and send `project_id` / `project_ids`. They also send `box_id` / `box_ids` with the same value, for servers from before the rename. If a server answers a projects path with a 404 that is not an API error (it has no such route), the CLI retries on the `/api/v1/boxes/...` path and uses the box paths for the rest of the run.
- Responses are read as `project` first, then `box`.
- `.missionbase/project.json` is read as `project_id` / `project_name`, then `box_id` / `box_name`; `project link` writes both pairs (see [Agent work](#agent-work-herdr-agents)).

### Agent work (Herdr agents)

These commands let orchestrators and their Claude Code / Codex workers track work on Missionbase tasks. They print short text by default; add `--json` for the server's JSON (or, for `tasks ls`, `tasks changes`, `task next` and `project`, a shaped JSON object described below).

```bash
missionbase-agent task start <task-id> [--terminal [machine/]tN] [--force] [--json]
missionbase-agent task stop <task-id> [--reason done|manual|reopened|reassigned|terminal_gone|agent_exited] [--json]
missionbase-agent task attention <task-id> "Short or long copy?" [--json]
missionbase-agent task attention <task-id> --body-file /tmp/question.md [--json]
missionbase-agent task attention clear <task-id> [--json]
missionbase-agent task ask <task-id> "Short or long copy?" [--context TEXT] [--option TEXT]... [--json]
missionbase-agent task ask <task-id> --file questions.json [--json]
missionbase-agent task questions <task-id> [--status open|answered|dismissed[,...]] [--json]
missionbase-agent task questions <task-id> --wait [--timeout DURATION] [--json]
missionbase-agent task next [--cwd DIR | --project ID] [--json]
missionbase-agent tasks ls [--all] [--status open|done|canceled|all|STATUS[,STATUS]] [--needs-you] [--cwd DIR | --project ID] [--json]
missionbase-agent tasks changes [--since CURSOR] [--project ID[,ID]] [--json]
missionbase-agent project [show] [--cwd DIR | --project ID] [--json]
missionbase-agent project link <project-id> [--cwd DIR]
missionbase-agent project default [ID | --clear]
missionbase-agent project watch [--terminal tN | --off] [--cwd DIR | --project ID] [--json]
```

- **`task start`** assigns the acting agent to the task, records where the work runs (hc core id, machine name, terminal) and moves Backlog/To Do to In Progress (`POST /api/v1/tasks/:id/work`); a Planning task stays in Planning. Without `--terminal` it uses `$HC_TERMINAL_ID` on this machine. `--terminal t7` is a terminal on this machine; `--terminal omarchy-intel/t7` is a terminal on a machine from `hc machine ls` (matched by name, host, target or core id). The core id comes from `hc server status --json` / `hc machine ls --json`; `hc` is only run by `task start`, with a 3 second timeout. If another agent is already running the task elsewhere, it exits 3 and says who is running it where; `--force` takes the work over.
- **`task stop`** clears where your work runs and keeps you assigned (`DELETE /api/v1/tasks/:id/work`). "Nothing of yours was running" is not an error.
- **`task attention`** asks the reason as one question (a shortcut for `task ask` with one question and no options). `clear` dismisses the task's open questions. Neither changes the task's status.
- **`task ask`** and **`task questions`** ask the task's owner questions and read the answers. See [Agent questions](#agent-questions).
- **`tasks ls`** lists the current project's tasks, open by default, in status order and then project order (`GET /api/v1/tasks/accessible?project_ids=<id>`, all pages). `--status` takes a category (`open`, `done`, `canceled`), `all`, or status keys such as `planning` or `todo,in_progress`. `--needs-you` keeps only the tasks waiting on their owner (open questions or a live permission prompt; `needs_you=true`), and the text output marks every such task "needs you" (with the oldest open question when there is one). `--status needs_attention` is refused with a pointer to `--needs-you`. `--all` lists every project you can access, ordered by project name (the same endpoint without `project_ids`). JSON: `{"project": {...} | null, "tasks": [...], "cursor": "..."}`. `cursor` is a changes-feed cursor taken before the list was read: the first page's `meta.cursor`, which is the earliest. It is `null` only if the server sent none.
- **`tasks changes`** returns tasks changed since a cursor (open and closed), following pages until done, deduped by id, with deleted tasks removed. Treat the cursor as opaque. Without `--since` it starts from the beginning. JSON: `{"tasks": [...], "deleted_task_ids": [...], "cursor": "..."}`.
- **`task next`** asks Missionbase for the next task in the current project (`GET /api/v1/projects/:id/tasks/next`): the first To Do task in project order that nobody is running and that is unassigned or assigned to you. In Progress, Planning and Backlog tasks, and tasks with open questions, are never picked. With no project it returns your next assigned task from `GET /api/v1/agent/work`, which never returns a Planning task or one with open questions. JSON: `{"has_work": bool, "task": {...} | null, "project": {...} | null}`.

**Keeping a task list current** (the Herdr panel's intended pattern): run `tasks ls --json` (or `tasks ls --all --json`) once, keep its `cursor`, then poll `tasks changes --since <cursor> --json`. Keep each response's new `cursor` for the next poll. Apply `tasks` by id (latest wins) and remove `deleted_task_ids`. Nothing changed after the list is missed. Some tasks come back once more because of the cursor's 5-second overlap, so dedupe by id. `tasks changes` returns open and closed tasks for every project you can access (narrow it with `--project ID[,ID]`), so filter to what you show.

```bash
missionbase-agent tasks ls --json > /tmp/tasks.json           # {"project", "tasks", "cursor"}
cursor=$(jq -r .cursor /tmp/tasks.json)
missionbase-agent tasks changes --since "$cursor" --json      # {"tasks", "deleted_task_ids", "cursor"}
```

**Project resolution** ("here") is the same for `tasks ls`, `task next` and `project`:

1. `--project ID`
2. the nearest `.missionbase/project.json` (its `project_id`, or `box_id` in a file written before the rename) from `--cwd DIR` or the current folder, searching parent folders
3. this machine's default project, set with `missionbase-agent project default ID` and stored as `default_project` in `~/.config/missionbase-agent/credentials`

`project link <project-id>` writes `.missionbase/project.json` in `--cwd DIR` (or the current folder) with the project's name and team from the server, keeping any other keys in the file: `{"team_id", "team_name", "project_id", "project_name", "box_id", "box_name"}`. The `box_*` pair is for CLIs and Herdr panels from before the rename. The `project` object in `--json` output also carries both pairs: `{"project_id", "project_name", "box_id", "box_name", "source", "path"}`.

**Watching a project** (for orchestrators): `project watch --terminal t9d` signs up terminal `t9d` on this machine for the current project's activity. The [missionbase-herdr](https://github.com/Quantum-Fire-Labs/missionbase-herdr) bridge then types one short digest into that terminal when its agent is idle: the owner's (users', not agents') new tasks, replies, question answers and status changes in the project, and replies that @mention the watching agent from anyone. A project has one watching terminal per machine: signing up again replaces it. `project watch --off` stops; `project watch` shows the project's watch (or every watch here when no project resolves). Nothing is stored in Missionbase: the watch lives in `~/.config/missionbase-herdr/watches.json` (`$XDG_CONFIG_HOME`, or `$MISSIONBASE_HERDR_WATCHES`), written in one rename:

```json
{"version": 1, "watches": [{"project_id": 5, "project_name": "Missionbase", "terminal": "t9d", "agent_slug": "claude", "since": "2026-11-13T19:40:00Z"}]}
```

`agent_slug` is the acting agent (`MISSIONBASE_AGENT_SLUG` or the folder config), so the bridge can spot mentions of it. `--terminal` takes only `tN`: the file is this machine's, so a terminal elsewhere is refused. Signing up checks that the agent can read the project (`GET /api/v1/projects/:id`) and stores its name.

Task payloads include `running` (agent, machine id/name, terminal, since, stale), `live_status`, `needs_you` (open questions or a live permission prompt), `attention` (derived from the oldest open question: reason, comment id (always null), since; null when none are open) and `questions` (`open_count`, `latest_batch_at`). The text output of `tasks ls`, `tasks changes` and `task next` shows "2 questions" when a task has open questions.

#### Agent questions

An agent can ask the task's owner 1 to 5 questions at a time. The task's status doesn't change: it reports `needs_you: true` while questions are open, and the owner answers (or dismisses) them on the task page. When the last open question is resolved the agent gets a `questions_answered` update (`listen`, or the `task.questions_answered` webhook). Agents can't answer or dismiss questions.

```bash
# One question, with optional context (Markdown) and up to 4 suggested answers
missionbase-agent task ask 42 "Should a 409 from Optimus count as delivered?" \
  --context "Optimus returns 409 when it already processed that event ID." \
  --option "Yes, treat as delivered" --option "No, keep as failure"

# Several questions from a file
missionbase-agent task ask 42 --file questions.json

# Block until they are answered (exit 6 if still open after 30 minutes)
missionbase-agent task questions 42 --wait --timeout 30m
```

`--file` is a JSON array of 1 to 5 objects. `body` is required; `context` and `options` (at most 4 non-blank strings) are optional. Other keys are rejected, so a typo doesn't send an empty question. Use either a question argument (with `--context`/`--option`) or `--file`, not both.

```json
[
  {"body": "Should a 409 from Optimus count as delivered?",
   "context": "Optimus returns 409 when it already processed that event ID.",
   "options": ["Yes, treat as delivered", "No, keep as failure"]},
  {"body": "Cap total retry time at 1 hour or 24 hours?", "options": ["1 hour", "24 hours"]}
]
```

`task ask` prints the questions it asked, the task's new status and a link to the task page:

```text
Asked 2 questions on #42 "Webhook retries"
  1. Should a 409 from Optimus count as delivered?
     Context: Optimus returns 409 when it already processed that event ID.
     Options: Yes, treat as delivered · No, keep as failure
  2. Cap total retry time at 1 hour or 24 hours?
     Options: 1 hour · 24 hours
Status: In Progress (unchanged; the task needs you until the questions are answered)
https://dash.missionbase.app/tasks/42
Wait for answers with: missionbase-agent task questions 42 --wait
```

`--json` prints the server's response (`batch`, `questions`, `task`). The CLI checks the counts, blank bodies and options before sending. The server's 422 errors are shown with their per-question details (indexes are 0-based):

```text
missionbase-agent: Invalid questions (HTTP 422 validation_error)
  questions[1]: Body is too long (maximum is 10000 characters)
```

A Complete or Not Doing task is refused with `Task is closed; reopen it before asking questions (HTTP 422 task_closed)`. Closing a task dismisses its open questions.

`task questions` lists every question on the task (`GET /api/v1/tasks/:id/questions`), grouped by the ask they came from, with its status and the answer or dismissal. `--status` takes `open`, `answered` and `dismissed`, comma-separated (`--open` is short for `--status open`). `--json` prints `{"task_id", "open_count", "questions"}`.

```text
#42: 2 questions, 0 open
Asked by Publisher at 2026-11-13T05:12:40Z
  1. [answered] Should a 409 from Optimus count as delivered?
     Options: Yes, treat as delivered · No, keep as failure
     Answer (John Doe): Yes, treat as delivered
  2. [dismissed] Cap total retry time at 1 hour or 24 hours?
     Options: 1 hour · 24 hours
     Dismissed by John Doe
```

**Images in answers.** The owner can answer with images, and an image alone is an answer. `task questions` (and `--wait`) saves an answer's images to `~/.cache/missionbase/answers/<task-id>/` (`$XDG_CACHE_HOME/missionbase/answers/<task-id>/` when set), named `q<question-id>-<n>-<filename>`, and prints their paths under the answer. Look at them before acting on the answer. An image-only answer reads `[2 images]`. Images already saved are reused. The download uses the image's signed URL, without the API key. If one can't be saved, its URL is printed instead and the command still succeeds.

```text
  1. [answered] Which layout should the settings page use?
     Answer (John Doe): This one
     This answer includes 2 images. Look at them before acting: /home/me/.cache/missionbase/answers/42/q7-1-layout.png, /home/me/.cache/missionbase/answers/42/q7-2-mobile.png
  2. [answered] Can you share a screenshot of the bug?
     Answer (John Doe): [1 image]
     This answer includes 1 image. Look at it before acting: /home/me/.cache/missionbase/answers/42/q8-1-bug.png
```

Each question in the API carries `answer_attachments: [{"filename", "content_type", "byte_size", "url"}]`. With `--json`, `task questions` adds `local_path` to each saved image (or `download_error` when it couldn't be saved).

`task questions --wait` polls every 20 seconds until the task has no open questions, then prints the questions that were open when it started. If none were open (the owner answered before the first poll), it prints the latest ask. It exits 0 when they are resolved. `--timeout` takes a duration (`90s`, `10m`, `1h`) or a number of seconds; without it, it waits indefinitely. On timeout it exits 6 (with `--json`, after printing the last response). Network errors, HTTP 429 and 5xx responses are retried on the next poll; other API errors stop it with exit 1. A manual status change also dismisses open questions, so `--wait` finishes then too: check each question's status.

`task show` prints the task detail JSON, which includes every question in `questions.items`.

#### Exit codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | Any other error: bad arguments, network failure, or an API error other than 409 |
| 3 | Conflict: the API answered HTTP 409 (for example `task start` while another agent runs the task elsewhere). Applies to every command. |
| 4 | `hc` is missing, failed or timed out, `$HC_TERMINAL_ID` is not set, or the machine in `--terminal machine/tN` is unknown |
| 5 | No project resolved (`tasks ls` and `project` without `--project`, a `.missionbase/project.json` or a default project) |
| 6 | `task questions --wait --timeout` ran out with questions still open |

### Local Pi sessions

List active agents available to the authenticated team token, then launch Pi with an explicit Missionbase agent identity:

```bash
missionbase-agent pi agents
missionbase-agent pi agents --json
missionbase-agent pi --agent missionbase-dev
missionbase-agent pi --agent chief --continue
missionbase-agent pi --agent chief --resume
missionbase-agent pi --agent chief --session 019fa64c
missionbase-agent pi --agent fleet-architect -- --model gpt-5.6
```

Use `--continue`/`-c` to open the most recent Pi session for the current working directory, `--resume`/`-r` to open Pi's interactive session chooser, or `--session SESSION`/`-s SESSION` to open a specific session path or ID. Exit any Pi process using that session before reopening it.

After enrolling the computer with the one-time `missionbase-agent connect TOKEN` command from Missionbase, the launcher synchronizes the selected Pi session's complete active transcript. Existing and subsequent visible user, assistant, tool, shell, and summary entries appear in Missionbase. Messages sent from Missionbase are injected as normal Pi user messages and are durably deduplicated across reconnects.

The launcher verifies the selected agent with Missionbase before starting Pi, preserves Pi's normal working-directory session behavior, and forwards arguments after `--` to Pi unchanged. Every child `missionbase-agent` command inherits the selected slug. The user-acting `missionbase` CLI refuses API commands inside the launched process so Missionbase writes cannot accidentally be attributed to the signed-in human.

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
missionbase scratchpad show
missionbase scratchpad update --body TEXT
missionbase scratchpad edit --body-file PATH
missionbase teams
missionbase users lookup <query-or-mention> [--team <team-id>]
missionbase team show <team-id>
missionbase team members <team-id>
missionbase projects [--team <team-id>]
missionbase project show <project-id>
missionbase projects tasks <project-id> [--status STATUS[,STATUS]] [--status-category open|done|canceled] [--page N] [--per-page N]
missionbase projects discussions <project-id> [--page N] [--per-page N]
missionbase projects discussions create <project-id> --title TITLE --body TEXT
missionbase projects documents create <project-id> --title TITLE --body TEXT
missionbase projects files <project-id> [--query QUERY] [--filter all|docs|files] [--sort newest|name|type] [--page N] [--per-page N] [--folder-id FOLDER_ID|--folder FOLDER_ID|--root] [--recursive]
missionbase projects files show <project-id> <file-id>
missionbase projects files mkdir <project-id> --title TITLE [--folder FOLDER_ID|--root]
missionbase projects files mv <project-id> <file-id> (--folder FOLDER_ID|--root)
missionbase projects files upload <project-id> --file PATH [--title TITLE] [--description TEXT] [--folder FOLDER_ID|--root]
missionbase projects files update <project-id> <file-id> [--title TITLE] [--description TEXT] [--folder FOLDER_ID|--root]
missionbase projects files message <project-id> <file-id> --body TEXT [--attach PATH]
missionbase projects files versions <project-id> <file-id>
missionbase projects files upload-version <project-id> <file-id> --file PATH
missionbase projects files download <project-id> <file-id> --output PATH [--version VERSION_ID]
missionbase projects task-statuses <project-id>
missionbase projects statuses <project-id>
missionbase notes search <query> [--team <team-id>]
missionbase scratchpad show
missionbase scratchpad fetch
missionbase scratchpad update --body TEXT
missionbase scratchpad edit --body-file PATH
missionbase sidebar pins
missionbase sidebar pin --type box_file --id <box-file-id>
missionbase sidebar unpin --type box_file --id <box-file-id>
missionbase document show <document-id> [--format markdown|html|plain-text]
missionbase document message <document-id> --body TEXT [--attach PATH]
missionbase document update <document-id> [--title TITLE] --body TEXT
missionbase tasks assigned [--page N] [--per-page N]
missionbase tasks visible [--page N] [--per-page N]
missionbase task create --title TITLE [--project ID] [--body TEXT] [--deadline YYYY-MM-DD] [--status STATUS] [--assign-user ID] [--attach PATH] [--attach-blob SIGNED_ID_OR_SGID]
missionbase task update <task-id> [--title TITLE] [--project ID] [--status STATUS]
missionbase task status <task-id> <status>
missionbase task complete <task-id>
missionbase task message <task-id> --body TEXT [--attach PATH] [--attach-blob SIGNED_ID_OR_SGID]
missionbase task assign <task-id> --user <user-id-or-mention> [--team <team-id>]
missionbase task unassign <task-id> --user <user-id-or-mention> [--team <team-id>]
missionbase task participants list <task-id>
missionbase task participants add <task-id> --user <user-id-or-mention> [--team <team-id>]
missionbase week show --team TEAM_ID [--starts-on YYYY-MM-DD]
missionbase week add TASK_ID --team TEAM_ID --starts-on YYYY-MM-DD
missionbase week remove PLACEMENT_ID --team TEAM_ID --starts-on YYYY-MM-DD
missionbase week order PLACEMENT_ID --team TEAM_ID --starts-on YYYY-MM-DD [--after-task TASK_ID]
missionbase today # signed-in user's current day in their time zone
missionbase day show --date YYYY-MM-DD
missionbase day add TASK_ID --date YYYY-MM-DD
missionbase day remove PLACEMENT_ID --date YYYY-MM-DD
missionbase day order PLACEMENT_ID --date YYYY-MM-DD [--after-task TASK_ID]
missionbase task show <task-id>
missionbase task messages <task-id> [--limit N]
missionbase task comments <task-id> [--limit N] # compatibility alias
missionbase conversations [--page N] [--per-page N]
missionbase discussion show <discussion-id> [--limit N]
missionbase discussion message <discussion-id> --body TEXT [--attach PATH] [--attach-blob SIGNED_ID_OR_SGID]
missionbase conversation show/message ... # deprecated aliases for discussion show/message
missionbase get /api/v1/users/me
missionbase post /api/path --json JSON
missionbase patch /api/path --json JSON
missionbase delete /api/path
missionbase update

Planning commands above act only as the authenticated user. Week dates must be Mondays; a week can be any date. `show` gives each task's `placement_id` for `remove` and `order`. Without `--after-task`, `order` moves a task to the first position. `today` gets the signed-in user's current date from the server in the user's time zone. Day commands always use the signed-in user's own day. Agent credentials cannot use these user CLI commands, and an agent-locked Pi session remains blocked by `MISSIONBASE_ACTOR_MODE=agent`.

missionbase-agent version
missionbase-agent auth status
missionbase-agent auth set-token <team-token> [--base-url URL] [--agent slug]
missionbase-agent use <agent-slug> [--base-url URL]
missionbase-agent pi agents [--json]
missionbase-agent pi --agent <agent-slug> [--continue | --resume | --session SESSION] [-- PI_ARGS...]
missionbase-agent me
missionbase-agent work [--next|--next-task]
missionbase-agent scratchpad show --user USER
missionbase-agent scratchpad edit --user USER --body-file PATH
missionbase-agent listen [--timeout N] [--offset ID] [--once]
missionbase-agent dm list [--limit N]
missionbase-agent dm show <chat-id>
missionbase-agent dm send --to <handle> --body-file /tmp/body.md
missionbase-agent dm send --chat <chat-id> --body-file /tmp/body.md
missionbase-agent agent create --name "Fleet Worker" --slug fleet-worker [--title "Fleet Architect"] [--description "Handles fleet tasks"] [--image avatar.png]
missionbase-agent agent image set fleet-worker --file avatar.png
missionbase-agent agent image clear fleet-worker --yes
missionbase-agent agent archive fleet-worker --yes
missionbase-agent agent restore fleet-worker --yes
missionbase-agent agent projects add fleet-worker --project <project-id> [--project <project-id>]
missionbase-agent agent instructions show fleet-worker
missionbase-agent agent instructions publish fleet-worker --body-file /tmp/instructions.md [--title "Managed instructions"]
missionbase-agent agent instructions activate fleet-worker <version-id>
missionbase-agent document show <document-id> [--format markdown|html|plain-text]
missionbase-agent document fetch <document-id> [--format markdown|html|plain-text] # compatibility alias
missionbase-agent document message <document-id> --body-file /tmp/reply.md [--attach /path/to/image.png]
missionbase-agent document create --project <project-id> --title "Doc title" --body-file /tmp/document.md [--folder FOLDER_ID|--root]
missionbase-agent document edit <document-id> [--title "New title"] --body-file /tmp/document.md
missionbase-agent tasks
missionbase-agent tasks --user <user-id-or-mention> [--scheduled actionable|future|all]
missionbase-agent week show [--starts-on YYYY-MM-DD] # read only; Monday; defaults to current week
missionbase-agent task show <task-id>
missionbase-agent task create --title "Task title" --project <project-id> [--deadline YYYY-MM-DD] [--scheduled-at DATETIME] [--assign-agent <agent-slug> | --assign-user <user-id-or-mention>] [--body-file /tmp/body.md] [--participant-user <user-id-or-mention>] [--attach /path/to/image.png] [--attach-blob <signed-id-or-sgid>]
missionbase-agent task update <task-id> --title "New task title"
missionbase-agent task update <task-id> --body-file /tmp/description.md # replaces the task description
missionbase-agent task update <task-id> --deadline YYYY-MM-DD
missionbase-agent task update <task-id> --no-deadline
missionbase-agent task update <task-id> --scheduled-at DATETIME
missionbase-agent task update <task-id> --no-scheduled-at
missionbase-agent task assign <task-id> --user <user-id-or-mention>
missionbase-agent task assign <task-id> --agent <agent-slug>
missionbase-agent task unassign <task-id> --user <user-id-or-mention>
missionbase-agent task unassign <task-id> --agent <agent-slug>
missionbase-agent task unassign <task-id> --self
missionbase-agent task message <task-id> --body-file /tmp/body.md [--attach /path/to/image.png]
missionbase-agent task status <task-id> <status>
missionbase-agent task move <task-id> --project <project-id>
missionbase-agent task complete <task-id>
missionbase-agent task messages <task-id> [--limit N]
missionbase-agent task comments <task-id> [--limit N] # compatibility alias
missionbase-agent task participants list <task-id>
missionbase-agent task participants add <task-id> --user <user-id-or-mention>
missionbase-agent task participants add <task-id> --agent <agent-slug>
missionbase-agent discussion show <discussion-id> [--limit N]
missionbase-agent discussion message <discussion-id> --body-file /tmp/body.md [--attach /path/to/image.png]
missionbase-agent conversation show/message ... # deprecated aliases for discussion show/message
missionbase-agent members [--project ID]
missionbase-agent activity <project|team> <id> [--since TIME|--duration DURATION] [--until TIME] [--actor TYPE:ID] [--subject-type TYPE] [--subject-id ID] [--action ACTION] [--cursor ID] [--limit N] [--json]
missionbase-agent projects activity <project-id> [same filters]
missionbase-agent projects create --name "Project Delivery" --team <team-id> [--description-file /tmp/description.md] [--kind project] [--status in_progress] [--visibility team]
missionbase-agent projects tasks <project-id> [--status STATUS[,STATUS] | --status-category open|done|canceled] [--scheduled actionable|future|all] [--page N] [--per-page N]
missionbase-agent projects notes show [project-id] [--json]
missionbase-agent projects notes edit [project-id] --body-file /tmp/notes.md [--base-version VERSION_ID] [--json]
missionbase-agent projects notes append [project-id] --body-file /tmp/entry.md [--date YYYY-MM-DD] [--json]
missionbase-agent projects notes history [project-id] [--limit N] [--before VERSION_ID] [--json]
missionbase-agent projects discussions <project-id> [--page N] [--per-page N]
missionbase-agent projects discussions create <project-id> --title TITLE --body-file /tmp/body.md
missionbase-agent projects files <project-id> [--query QUERY] [--filter all|docs|files] [--sort newest|name|type] [--page N] [--per-page N] [--folder-id FOLDER_ID|--folder FOLDER_ID|--root] [--recursive]
missionbase-agent projects files show <project-id> <file-id>
missionbase-agent projects files mkdir <project-id> --title TITLE [--folder FOLDER_ID|--root]
missionbase-agent projects files mv <project-id> <file-id> (--folder FOLDER_ID|--root)
missionbase-agent projects files upload <project-id> --file PATH [--title TITLE] [--description TEXT] [--folder FOLDER_ID|--root]
missionbase-agent projects files update <project-id> <file-id> [--title TITLE] [--description TEXT] [--folder FOLDER_ID|--root]
missionbase-agent projects files message <project-id> <file-id> --body-file /tmp/reply.md [--attach /path/to/image.png]
missionbase-agent projects files versions <project-id> <file-id>
missionbase-agent projects files upload-version <project-id> <file-id> --file PATH
missionbase-agent projects files download <project-id> <file-id> --output PATH [--version VERSION_ID]
missionbase-agent sidebar pins --user <user-id-or-mention>
missionbase-agent sidebar pin --user <user-id-or-mention> --type box_file --id <box-file-id>
missionbase-agent sidebar unpin --user <user-id-or-mention> --type box_file --id <box-file-id>
missionbase-agent projects task-statuses <project-id>
missionbase-agent projects statuses <project-id>
missionbase-agent get /api/v1/agent/me
missionbase-agent update
```

User CLI read and write commands return raw JSON directly from the Missionbase API. `missionbase me` calls `/api/v1/users/me` only, `missionbase work` calls `/api/v1/users/work` to return the current user, assigned/open tasks, unread discussion conversations, unread direct-message conversations, and inbox-oriented metadata, user/mention lookup uses `/api/v1/users/lookup` and `/api/v1/teams/:team_id/members`, and assignment/participant writes use user-acting task endpoints. The user CLI never calls agent-only member endpoints, never supports `--agent` assignment/participants, and agent identity and agent-management workflows remain exclusive to `missionbase-agent`; human DM unread state is exposed only through the user work overview JSON.

User CLI write commands use JSON requests when no attachments are present and multipart requests when repeated `--attach PATH` or `--attach-blob SIGNED_ID_OR_SGID` flags are used. Local attachments are limited to PNG, JPEG, GIF, and WEBP images up to 5 MB. User-authored task descriptions, task/discussion/document/file messages, and box discussion bodies are Markdown-capable and normalize accidental escaped newline sequences outside quoted/code contexts. Message body aliases `--body`, `--comment`, `--message`, and `--text` are supported.

Sidebar pin commands manage pinned sidebar pages as JSON. `missionbase sidebar pins`, `missionbase sidebar pin --type box_file --id ID`, and `missionbase sidebar unpin --type box_file --id ID` always act as the signed-in user and do not accept a target user. `missionbase-agent sidebar pins --user ID|@mention`, `missionbase-agent sidebar pin --user ID|@mention --type box_file --id ID`, and `missionbase-agent sidebar unpin --user ID|@mention --type box_file --id ID` use agent authentication and permissions to manage a specific user's pins. The initial supported item type is `box_file`, where `ID` is the BoxFile/Docs & Files entry id.

`missionbase users lookup <query>` calls user lookup directly. `missionbase users lookup @mention --team <team-id>` resolves a team member mention. Task assignment and participant commands accept numeric user ids directly; when resolving `@mention`, pass `--team` or let the CLI derive the team from the task when the task response includes box/team context. If team context cannot be inferred, the CLI asks for `--team` or a numeric user id.

`missionbase-agent activity project <project-id>` and `missionbase-agent projects activity <project-id>` query agent-visible project activity from `/api/v1/projects/:project_id/activity_events`; `missionbase-agent activity team <team-id>` queries `/api/v1/teams/:team_id/activity_events`. Activity output is concise text by default, including timestamp, scope, actor, action, subject, summary, source, route, useful metadata, and `Next cursor` when more results may be available. Add `--json` to print the raw API response for tooling. Filters include `--since`, `--until`, `--duration`/`--last`, `--actor TYPE:ID`, `--actor-type`, `--actor-id`, `--subject-type`, `--subject-id`, `--action`, `--cursor`, and `--limit`; the API enforces normal box/team permissions and only returns accessible activity.

`missionbase-agent projects notes ...` reads and writes a project's notes: one Markdown page per project, with version history (`/api/v1/projects/:id/notes`). Without a project id it uses `--project`, then the nearest `.missionbase/project.json` (from `--cwd` or the current folder), then the machine's default project (exit code 5 when none resolves). `show` prints the Markdown, preceded by a `<!-- version N: who what -->` line. `edit` replaces the whole page; pass `--base-version` with the version you read to keep entries appended since then, and a different change since then is refused (HTTP 409, exit code 3). `append` adds a dated entry under `## Log` (newest first, under the day's `### Nov 13, 2026` heading, today on this machine unless `--date`) as its own version attributed to the agent, without rewriting the rest of the page. `history` lists versions newest first: id, time, who and what, and +/- line counts.

`missionbase-agent projects create ...` creates a team-owned project and prints the created project JSON. `--name` and `--team` are required; optional kind, status, and visibility values are validated by Missionbase. Use `--description-file` for a Markdown description.

`missionbase-agent projects discussions ...` lists standalone box discussions only; it does not include task conversations. `missionbase-agent projects discussions create ...` creates a standalone box discussion with an initial message and prints the created discussion JSON. `missionbase-agent projects files ...` lists/searches unified BoxFile-backed Docs & Files items with identifiers, canonical preview URLs, fetch/download metadata, creator/owner, timestamps, status, content type, size, filename, folder parent/path/children metadata, filter/sort, and pagination metadata. Use `--folder-id FOLDER_ID` (or legacy `--folder FOLDER_ID`) or `--root` to scope listings/uploads, `--recursive` to search across folders, `mkdir` to create folders, and `mv`/`update --folder|--root` to move docs/files/folders. `show ...` prints one BoxFile/document/folder entry, `upload ... --file PATH` adds a file upload, `update ...` edits uploaded file/folder metadata, and `download ... --output PATH` writes an uploaded file to disk (folders are rejected by the API). User-facing `missionbase projects files ...` supports the same list/show/upload/update/download workflows while acting as the signed-in user. `missionbase-agent document show ...` prints a document body and reports the document URL when the API response includes one; `document fetch` remains a compatibility alias; `--format` accepts `markdown` (default), `html`, or `plain-text`. `missionbase-agent document create ...` creates a box document and prints the created document JSON, including its URL and any backing BoxFile metadata returned by the API; pass `--folder FOLDER_ID` or `--root` to place the document in a folder or at the box root. `missionbase-agent document edit ...` updates an existing document by creating a new document version. `missionbase-agent week show ...` reads the selected team week through `/api/v1/weeks`; it returns only tasks in boxes the agent can access. The date must be a Monday. It does not create a week. `missionbase-agent task show ...` prints full task working context from `/api/v1/tasks/:id`. `missionbase-agent task message ...` posts a message/reply to the task discussion. `missionbase-agent discussion message ...` posts by canonical discussion id, while `document message ...` and `boxes files message ...` resolve their canonical document/file ids to the backing discussion before posting. `missionbase-agent conversation show/message` remains only as a deprecated alias for `discussion show/message`.

Task, discussion, document, file, box discussion create, and DM bodies are Markdown-capable by default; Missionbase renders headings, bold/italic, inline code, fenced code blocks, bullet/numbered lists, blockquotes, and links as sanitized rich text while ordinary plain text continues to display normally. These agent-authored body fields also defensively normalize accidental escaped newline sequences (`\\n`, `\\r`, and `\\r\\n`) into real line breaks outside quoted/backticked code contexts.

Agent-authored posting bodies are file-only: use `--body-file PATH` for DM bodies, task messages, discussion/document/file messages, box discussion bodies, and document bodies. Task descriptions use `--body-file PATH` on `task create` and `task update`. Inline body flags and stdin body input (`--body-stdin` or `--body-file -`) are intentionally unsupported for these write flows so Markdown, backticks, and shell-sensitive content are read from disk instead of passing through fragile shell quoting or piped interactive flows.

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

missionbase-agent task message 123 --body-file /tmp/missionbase-comment.md
```

When file content accidentally contains escaped newline sequences (`\n`, `\r`, or `\r\n`), the CLI continues to normalize them to real line breaks outside quoted/backticked code contexts. Quoted JSON, shell snippets, and inline-code literals such as `printf 'a\\nb'` are preserved.

`missionbase-agent task create --scheduled-at DATETIME` and `missionbase-agent task update <task-id> --scheduled-at DATETIME` set `scheduled_at` separately from `deadline`; use `--no-scheduled-at` (or `--clear-scheduled-at`) to clear scheduling without changing the deadline. The API parses schedule datetimes in the acting user's timezone when no offset is included, so include an ISO-8601 offset or `Z` for an absolute instant. Normal agent work/task endpoints keep the API default scheduled filter, hiding future scheduled tasks until actionable; use `--scheduled future` or `--scheduled all` on supported task listings only when explicitly discovering scheduled tasks.

`missionbase task assign ...` and `missionbase task unassign ...` manage user assignments for existing tasks with the user-acting CLI. Use `--user` with a numeric user id or `@mention`; mention resolution uses `--team <team-id>` or task-derived team context when available. `missionbase task participants list ...` lists task participants, and `missionbase task participants add ... --user ...` adds user participants only.

`missionbase-agent task assign ...` and `missionbase-agent task unassign ...` manage assignments for existing tasks using the Missionbase assignment API. Use `--user` with a numeric user id or `@mention`, `--agent` with an agent slug, or `task unassign <task-id> --self` to safely remove the currently selected agent from a task after handing it off.

`missionbase-agent task move <task-id> --project <project-id>` moves an existing task to another agent-accessible project through the task update API. Missionbase preserves the task record, discussion messages, attachments, participants, and assignments. The status is kept as it is, because every project has the same statuses.

`missionbase-agent projects task-statuses <project-id>` (alias: `projects statuses`) prints the task statuses as JSON. They are fixed and the same in every project: `backlog`, `planning`, `todo`, `in_progress` (open), `complete` (done) and `not_doing` (canceled). `needs_attention` was removed: "needs you" is the `needs_you` flag, and `task status <id> needs_attention` is refused with a pointer to `task ask`. `planning` means the task is being planned with its owner, not built: work out the plan (ask with `task ask`, write the plan into the task) and move it to `todo` with `task status <id> todo` once the plan is agreed. Each entry has `key`, `name`, `category`, `color`, `position`, `default_open`, `primary_done` and `primary_canceled`. There are no status ids; commands take status keys (`--status KEY`) or categories (`--status-category open|done|canceled`).

Task create/message and discussion/document/file message accept repeated `--attach PATH` flags for local image files and repeated `--attach-blob SIGNED_ID_OR_SGID` flags to reuse an existing Missionbase ActiveStorage blob from an attachment response. Supported local/blob attachment types are PNG, JPEG, GIF, WEBP, HEIC, and HEIF images up to 5 MB each. Attachments are appended inline to the task description or message rich text so they are visible in the Missionbase UI.

Examples:

```bash
missionbase-agent task create --project 2 --title "Investigate screenshot" --body-file /tmp/body.md --attach /tmp/screenshot.png
missionbase users lookup Daniel --team 2
missionbase task assign 123 --user @DanielLemky --team 2
missionbase task participants add 123 --user 42
missionbase-agent task create --project 2 --assign-agent missionbase-dev --title "Assigned investigation"
missionbase-agent task assign 123 --user @DanielLemky
missionbase-agent task unassign 123 --self
missionbase-agent task move 123 --project 45
missionbase-agent task message 123 --body-file /tmp/comment.md --attach /tmp/repro.webp
missionbase-agent activity project 2 --duration 24h --limit 25
missionbase-agent projects activity 2 --since 2026-07-01T00:00:00Z --action task.status_changed
missionbase-agent activity team 1 --actor Agent:2 --subject-type Task --subject-id 2902 --json
missionbase-agent projects discussions 2
missionbase-agent projects discussions create 2 --title "Release workflow planning" --body-file /tmp/proposal.md
missionbase-agent projects files 2
missionbase-agent projects files 2 --query runbook --per-page 25
missionbase-agent projects files mkdir 2 --title Assets
missionbase-agent projects files mv 2 77 --folder 42
missionbase-agent projects files 2 --folder-id 42
missionbase-agent document show 789
missionbase-agent document show 789 --format html
missionbase-agent document fetch 789 --format plain-text
missionbase-agent document create --project 2 --title "Runbook" --body-file /tmp/runbook.md --folder 42
missionbase-agent document edit 789 --title "Updated runbook" --body-file /tmp/runbook-v2.md
missionbase-agent discussion message 456 --body-file /tmp/reply.md --attach /tmp/context.png
missionbase-agent task message 123 --body-file /tmp/findings.md
missionbase-agent task message 123 --body-file /tmp/comment.md --attach-blob "<signed-id-or-sgid>"
```

### Agent management

`missionbase-agent agent create ...` creates a new agent on the authenticated team and prints the created agent JSON, including its id and slug. It requires a team API key with `agents:create` permission; invalid or duplicate slugs are returned as API validation errors.

`agent create --image PATH` also uploads a profile image for the new agent. `missionbase-agent agent image set AGENT --file PATH` sets or replaces an agent's profile image, and `agent image clear AGENT --yes` removes it (clearing an agent with no image succeeds with `"removed": false`). Images must be PNG, JPEG, or WEBP and at most 5 MB; the CLI checks the file contents before uploading and the server checks them again. The server resizes images to fit within 500×500 and stores them as JPEG, like user avatars, so transparency is not kept. An agent can change its own image when its slug is selected; changing another agent's image requires `agents:update`. Both commands print the updated agent JSON, and the new `profile_image_url` also appears in `me` and `pi agents --json`.

`missionbase-agent agent projects add ...` (old name: `agent boxes add`) adds an agent to one or more projects and prints JSON with the agent and membership status (`created` or `existing`) for each project. It requires `agents:update` and `projects:update` (or the old `boxes:update`) permissions.

`missionbase-agent agent instructions show AGENT` reads the active managed instruction version and requires `agents:read`. `agent instructions publish AGENT --body-file PATH [--title TITLE]` publishes and activates a new version, while `agent instructions activate AGENT VERSION_ID` reactivates an existing version; both writes require `agents:update`. Responses include the version id and number, active state, title, and exact content. Publish reads content directly from the required body file without newline normalization so callers can hash-verify the response. These operations do not change agent execution mode or provisioning state.

`missionbase-agent agent archive ... --yes` is the supported safe delete flow for agents. It archives/deactivates the agent instead of hard-deleting it, preserving historical task/comment/message attribution. Archived agents are removed from active assignment, mention, DM, and box membership choices; agent-owned API keys are revoked; and selected-agent credentials using the archived slug are rejected. The server refuses to archive an agent that is still assigned to open tasks (any status in the open category), so hand off or close that work first.

`missionbase-agent agent restore ... --yes` restores/reactivates an archived agent with its existing identity and box memberships so it can be assigned and used for new work again. Restoring does not recreate agent-owned API keys that were revoked during archival; create new credentials if the restored agent needs to authenticate.

```bash
missionbase-agent agent create --name "Fleet Worker" --slug fleet-worker --title "Fleet Architect" --description "Handles fleet tasks"
missionbase-agent agent image set fleet-worker --file ./fleet-worker.webp
missionbase-agent agent image clear fleet-worker --yes
missionbase-agent agent projects add fleet-worker --project 2
missionbase-agent agent projects add 42 --project 2 --project 7
missionbase-agent agent instructions show fleet-worker
missionbase-agent agent instructions publish fleet-worker --body-file /tmp/instructions.md --title "Chief runner instructions"
missionbase-agent agent instructions activate fleet-worker 42
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
- `questions_answered` — the owner resolved the last open question on a task this agent asked about. The payload has the task's new `status` and each resolved question with its `answer_body` or dismissal.

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

`missionbase-agent` prints the Missionbase API JSON response as-is for read commands. `missionbase-agent work` returns the next actionable assigned task as full task context, or a structured empty response (`{"has_work":false,"task":null}`). Discussions, mentions, and DMs are wake-up/read flows driven by `listen`, `discussion show`, and `dm show`, not by `work`. Task descriptions, task discussion messages, full conversation messages, and DM messages include backwards-compatible plain text fields plus rich text fields when the server provides them:

- `description`, `body`, or `content`: existing plain text or HTML-compatible field, depending on the command.
- `description_html`, `body_html`, or `content_html`: rendered rich-text HTML.
- `description_rich_text`, `body_rich_text`, or `content_rich_text`: object with `plain_text`, `html`, and `attachments`.
- `attachments`: convenience copy of the rich-text attachment list. File/image attachments include `filename`, `content_type`, `byte_size`, `image`, and a relative download `url` where supported.

Use `missionbase-agent task show <task-id>`, `missionbase-agent discussion show <discussion-id>`, `missionbase-agent task messages <task-id>`, `missionbase-agent document show <document-id>`, and `missionbase-agent dm show <chat-id>` to inspect this rich content. Agents should use the plain text fields for prompt context and consult the `attachments` arrays to discover files/images that may need separate handling.

### Other agent commands

`missionbase-agent members` lists group members, including mention handles/usernames to use when tagging humans or agents. `missionbase-agent task status <task-id> <status>` sets a task's status by key (`backlog`, `planning`, `todo`, `in_progress`, `complete`, `not_doing`); `task move <task-id> --project <project-id>` moves a task to another accessible box and keeps its status; `complete` is routed through Missionbase's complete endpoint so completion metadata and recurring follow-ups are handled correctly. `missionbase-agent task participants ...` adds and lists task participants through high-level commands. `missionbase-agent projects tasks <project-id>` lists open-category tasks in an accessible box by default; use `--status KEY[,KEY]`, `--status-category`, `--page`, and `--per-page` to refine results. The old `--task-status-ids` / `--task-status-id` flags were removed along with status ids and now fail with a pointer to `--status`. `missionbase-agent projects discussions <project-id>` lists standalone box discussions only, while `discussion show/message` is the generic canonical-id surface for standalone discussions and other readable discussion threads (`conversation show/message` is deprecated). `missionbase-agent projects files <project-id>` lists and searches unified box files/documents as JSON; use `--query`, `--filter`, `--sort`, `--folder-id`, `--root`, `--recursive`, `--page`, and `--per-page` to filter or paginate, plus `show`, `mkdir`, `mv`, `upload`, `update`, `versions`, `upload-version`, and `download` subcommands for file/folder workflows. Task payloads carry `status` (the key), `status_label`, `status_category`, `task_status_position` and `status_color`; box payloads list the fixed `task_statuses`. `get` is included as a low-level escape hatch while higher-level task/page/team commands are ported.

## Agent check helper

`scripts/missionbase-agent-check` is the local fleet check script used by timers on agent hosts. It currently runs `missionbase-agent work`, exits when `has_work` is false, and otherwise uses the single returned task for the Pi run. `missionbase-agent work --next`/`--next-task` are accepted as backward-compatible no-ops; the server always returns at most one actionable assigned open task. Selection prefers due scheduled tasks by `scheduled_at`, then unscheduled tasks by `created_at`.

Selected direct tasks use Pi session id `missionbase-task-<task_id>`. The script passes both `--session-id` and a descriptive `--name` to Pi. If the task box includes `working_directory`, fleet wrappers can invoke Pi from that absolute path.

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
