# Changelog

## Unreleased

- Added user CLI lookup, user assignment/unassignment, and task participant commands: `missionbase users lookup`, `missionbase task assign`, `missionbase task unassign`, `missionbase task participants list`, and `missionbase task participants add --user`.
- Added user mention resolution for numeric ids and team-scoped `@mention`s using only user-facing endpoints.
- Added Missionbase user CLI catch-up phase 1 read-only commands for teams, boxes, box tasks/discussions/statuses, user task listings, task feeds, conversations, and conversation feeds.
- Updated user CLI auth/request handling so user mode uses only user credentials and never sends `X-Missionbase-Agent-Slug` or reads agent directory config.
- Kept `missionbase me` user-only by removing the previous `/api/v1/agent/me` fallback.
