# Changelog

## Unreleased

- Added `missionbase pi --team TEAM ...` and `--one-shot` passthrough for the interactive runner launcher.
- Added managed agent-instruction commands: `missionbase-agent agent instructions show|publish|activate`, with exact body-file content preserved for hash verification.
- Added agent activity query tooling: `missionbase-agent activity <box|team> <id>` and `missionbase-agent boxes activity <box-id>` support time ranges, duration shortcuts, actor/subject/action filters, cursor pagination, concise default output, and `--json` raw output.
- Added canonical-id discussion messaging commands (`discussion show/message`) plus document/file message helpers; `conversation show/message` is now documented as a deprecated alias.
- Added scratchpad fetch/update commands to both CLIs: `missionbase scratchpad show|update|edit` and `missionbase-agent scratchpad show|edit --user USER`.
- Switched task creation opening text to `body`: `missionbase task create --body TEXT` and `missionbase-agent task create --body-file PATH`; task updates now only change task metadata such as deadline/schedule/status/box/title.
- Added Missionbase user CLI catch-up phase 5 notes/document/raw helpers: `missionbase notes search`, `missionbase boxes documents create`, `missionbase document show`, `missionbase document update`, plus clearly warned raw `post`/`patch`/`delete` signed-in-user helpers.
- Updated `missionbase --help` and README user CLI documentation to describe day-to-day Missionbase workflows beyond low-level API access.
- Added `missionbase work` for user-acting current-work overviews via `/api/v1/users/work`, including current user, assigned/open tasks, unread conversations, and metadata.
- Added Missionbase user CLI catch-up phase 3 lookup, user assignment/unassignment, and task participant commands: `missionbase users lookup`, `missionbase task assign`, `missionbase task unassign`, `missionbase task participants list`, and `missionbase task participants add --user`.
- Added user mention resolution for numeric ids and team-scoped `@mention`s using only user-facing endpoints.
- Added Missionbase user CLI catch-up phase 2 safe write commands for task create/update/status/complete/message, discussion messages, and box discussion creation.
- Added shared Markdown body normalization and attachment multipart helpers for user CLI writes, with PNG/JPEG/GIF/WEBP validation and a 5 MB file limit.
- Added Missionbase user CLI catch-up phase 1 read-only commands for teams, boxes, box tasks/discussions/statuses, user task listings, task discussion messages, conversations, and discussion conversations.
- Updated user CLI auth/request handling so user mode uses only user credentials and never sends `X-Missionbase-Agent-Slug` or reads agent directory config.
- Kept `missionbase me` user-only by removing the previous `/api/v1/agent/me` fallback.
