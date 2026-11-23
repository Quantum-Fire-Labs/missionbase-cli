# missionbase: Missionbase as the person

`missionbase` is the user CLI. It acts as the signed-in person: every task,
message, note and status change it makes is theirs, shown under their name.
Use it when you're working for the person (as their assistant, on their
request). To track your own work as an agent, use your agent wrapper instead
(`missionbase-claude`, `missionbase-codex`; see its `--skill`). This manual
comes with the CLI and matches its commands; `missionbase --help` lists every
command and flag.

## Acting for someone

- Do what they asked, as they would. Anything you post reads as written by
  them, so keep their voice and don't announce yourself.
- Mark a task complete only when they say so (`missionbase task complete <id>`).
- Before replacing anything (a note, a document, a task's description), read
  it first. Prefer the commands that add (`append`, `message`) over the ones
  that replace.
- Ask before doing something they'd want to decide: deleting, moving work
  between projects, or messaging other people.
- `missionbase auth status` and `missionbase me` show who you're signed in as.

## Their work

```sh
missionbase work                         # their current work overview
missionbase today                        # their day: the tasks placed on today
missionbase day show --date YYYY-MM-DD
missionbase day add <task-id> --date YYYY-MM-DD
missionbase tasks assigned               # tasks assigned to them
missionbase tasks visible                # every task they can see
```

`day remove` and `day order` take the placement id from `day show`.

## Tasks

```sh
missionbase task show <id>
missionbase task messages <id>
missionbase task create --title "..." --project <project-id> --body TEXT
missionbase task update <id> --title "..."
missionbase task status <id> <status>
missionbase task message <id> --body TEXT
missionbase task assign <id> --user @handle
missionbase task participants add <id> --user @handle
```

- Status keys: `backlog`, `todo`, `planning`, `in_progress`, `review`,
  `complete`, `not_doing`. To Do is the queue agents plan from;
  `missionbase task status <id> todo --session` has it planned one-on-one with
  the person in a Herdr tab instead of through Questions.
- `missionbase task create --title "..." --deadline YYYY-MM-DD --status todo --attach PATH`
  sets more at once; `missionbase task create --title "..." --start-on YYYY-MM-DD`
  makes a Backlog task that moves to To Do that day.
- Link other tasks in messages as
  `[#373 Voice screen](https://dash.missionbase.app/tasks/373)`.
- `missionbase users lookup` lists the people they can mention;
  `missionbase users lookup @handle` (or an email) finds one.

## Projects, discussions and files

```sh
missionbase projects                     # projects they can see
missionbase project show <project-id>
missionbase projects tasks <project-id> --status todo,in_progress
missionbase projects discussions <project-id>
missionbase projects files <project-id> --query TEXT
missionbase projects files download <project-id> <file-id> --output PATH
missionbase document show <document-id>
missionbase document update <document-id> --body TEXT
missionbase discussion message <discussion-id> --body TEXT
```

## Notes

Daily notes are one Markdown page per day; project notes are one page per
project. Both are free-form.

```sh
missionbase notes daily show             # today in their time zone; or a date
missionbase notes daily list
missionbase notes daily append --body-file PATH
missionbase notes daily edit 2026-11-22 --body-file PATH
missionbase notes project show <project-id>
missionbase notes project append <project-id> --body-file PATH
missionbase notes project edit <project-id> --body-file PATH --base-version ID
missionbase notes search <query>
```

- `append` adds the text to the end; prefer it.
- `edit` replaces the whole page: `show` it first and keep what's there. On
  project notes, pass the version you read as `--base-version`, which keeps
  text appended since.

## Activity

```sh
missionbase activity                     # today, across their projects
missionbase activity --date YYYY-MM-DD --project ID
```

It lists who did what, oldest first, in their time zone. A long day ends with
the command for the next page (`missionbase activity --cursor ID`).

## Raw helpers

`missionbase get /api/path` reads any API path. `post`, `patch` and `delete`
write as the person: use them only when no command above covers it.
