# Changelog

## Unreleased

- Added `missionbase-agent task update <task-id> --description-file PATH` for safely replacing task descriptions from Markdown/plain-text files.
- Added Missionbase user CLI catch-up phase 5 notes/document/raw helpers: `missionbase notes search`, `missionbase boxes documents create`, `missionbase document show`, `missionbase document update`, plus clearly warned raw `post`/`patch`/`delete` signed-in-user helpers.
- Updated `missionbase --help` and README user CLI documentation to describe day-to-day Missionbase workflows beyond low-level API access.
- Added `missionbase work` for user-acting current-work overviews via `/api/v1/users/work`, including current user, assigned/open tasks, unread conversations, and metadata.
- Added Missionbase user CLI catch-up phase 3 lookup, user assignment/unassignment, and task participant commands: `missionbase users lookup`, `missionbase task assign`, `missionbase task unassign`, `missionbase task participants list`, and `missionbase task participants add --user`.
- Added user mention resolution for numeric ids and team-scoped `@mention`s using only user-facing endpoints.
- Added Missionbase user CLI catch-up phase 2 safe write commands for task create/update/status/complete/comment, conversation comments, and box discussion creation.
- Added shared Markdown body normalization and attachment multipart helpers for user CLI writes, with PNG/JPEG/GIF/WEBP validation and a 5 MB file limit.
- Added Missionbase user CLI catch-up phase 1 read-only commands for teams, boxes, box tasks/discussions/statuses, user task listings, task feeds, conversations, and conversation feeds.
- Updated user CLI auth/request handling so user mode uses only user credentials and never sends `X-Missionbase-Agent-Slug` or reads agent directory config.
- Kept `missionbase me` user-only by removing the previous `/api/v1/agent/me` fallback.
