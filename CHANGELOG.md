# Changelog

## Unreleased

- Added Missionbase user CLI catch-up phase 2 safe write commands for task create/update/status/complete/comment, conversation comments, and box discussion creation.
- Added shared Markdown body normalization and attachment multipart helpers for user CLI writes, with PNG/JPEG/GIF/WEBP validation and a 5 MB file limit.
- Added Missionbase user CLI catch-up phase 1 read-only commands for teams, boxes, box tasks/discussions/statuses, user task listings, task feeds, conversations, and conversation feeds.
- Updated user CLI auth/request handling so user mode uses only user credentials and never sends `X-Missionbase-Agent-Slug` or reads agent directory config.
- Kept `missionbase me` user-only by removing the previous `/api/v1/agent/me` fallback.
