# {{cli}}: track your work in Missionbase

Missionbase (https://dash.missionbase.app) is where the owner sees and steers
agents' work: tasks, their statuses, questions for the owner, and chat. `{{cli}}`
acts as your Missionbase agent: everything it posts is signed by that agent.
Run the commands below as written. Never use the user CLI `missionbase` for
this: it acts as the person. This manual comes with the CLI and matches its
commands; `{{cli}} --help` lists every command and flag.

## Only in linked folders

Use Missionbase only when the working folder, or one of its parents, has
`.missionbase/project.json`. Anywhere else, don't create, start or message
tasks, and don't ask Missionbase Questions: do the work and ask in chat. The
exception is when the user explicitly asks you to use Missionbase there; then
pass `--project <id>`. A machine-wide default project doesn't count as linked.

```sh
{{cli}} project                    # which project "here" is, and where it came from
{{cli}} tasks ls --json            # this project's open tasks
```

## Statuses

- **Backlog:** not for now: not planned yet, or work that shouldn't go ahead at
  the moment. Don't work on it.
- **To Do:** the queue of work to plan. Agents pick it up with `task next`;
  `task start` moves it to Planning. A To Do task can be in session mode ("To Do
  · session with me"): it's planned one-on-one with the owner (see Planning).
- **Planning:** plan it with the owner. Don't build until the plan is approved.
- **In Progress:** being built: someone is working on it now, or it's waiting
  on Questions you asked for that work. When nobody is working on it and nothing
  is pending, move it to Backlog, not To Do. Don't leave it In Progress.
- **Review:** finished, verified and shipped if it needs shipping, waiting for
  the owner to look. Agents finish into it with `task review`.
- **Complete:** only the owner marks a task Complete, once they've looked at it
  in Review. Agents never do: the server refuses it.
- **Not Doing:** dropped.
- "Needs you" is not a status. It shows while a task has open questions or a
  permission prompt.

Change a status with `{{cli}} task status <id> <key>` (`backlog`, `todo`,
`planning`, `in_progress`, `not_doing`); `review` and `complete` are refused.

## The flow

1. **Find or create the task.** Reuse one from `{{cli}} tasks ls --json`, take
   the next one with `{{cli}} task next`, or create one:
   `{{cli}} task create --title "..." --project <project-id> --body-file PATH`.
   Work that's already agreed can start building at once:
   `{{cli}} task create --title "..." --status in_progress --start`.
2. **Start it:** `{{cli}} task start <id> --terminal $HC_TERMINAL_ID`. This
   assigns you, records where the work runs (the owner's dashboard shows it
   live) and moves Backlog or To Do to Planning. The terminal is where the
   owner's messages on the task are typed (see Task chat), so keep it accurate.
   When handing the task to another agent, use
   `{{cli}} task start <id> --terminal <its hc address>` and give it the id.
   Exit code 3 means it's running elsewhere: tell the user.
   `{{cli}} task start <id> --force` takes it over, only if they say so.
3. **Plan it** with the owner (see Planning). Finish by writing the plan into
   the task (`task message`) and asking one approval question: "Approve this
   plan and start building?", with the options *Approve and build* / *Change it*.
4. **Build it** once approved: `{{cli}} task status <id> in_progress`, then
   build it yourself right away. The agent that plans a task builds it.
5. **Hand it back** after verifying (and shipping, if it needs shipping):
   `{{cli}} task review <id> --item "LINE" [--item "LINE" ...] --body-file PATH`.
   The body is your completion message, and its first line is the result (e.g.
   "Live (337e9902): Review status shipped"). Each `--item` is one line on
   something the owner should check (1–10, e.g. "Home's Review row shows
   1 of 3"). For items that need detail (a link, steps), use
   `--items-file PATH` instead: a JSON array of `{"line", "detail"}`, the
   detail in Markdown. This moves the task to Review and ends your run. The
   owner checks each item: anything that needs work comes back as their message
   on the task (to the project's orchestrator), and the last one looking good
   marks the task complete.

If a task shouldn't go ahead now, move it to Backlog and say why. Once a task's
work has shipped, a follow-up is a new task that links to it, not more work on
the old one.

## Planning

- **Through Questions** (the default): ask the decisions that are the owner's
  to make (see Questions). Don't build until the plan is approved.
- **Session mode** (`planning_mode: session`; `task next` and `task start` say
  "· session with me"): the owner plans it with you one-on-one. Don't ask
  Questions. Run in a Herdr tab in the project's space
  (`herdr-app do terminal.keep --terminal <tN> --place tab`), not a background
  terminal. After `task start`, post a short opening on the task (your
  understanding, the main open decisions, your recommendation), then wait for
  the owner in your tab. End the same way: the plan in the task and one
  approval, which the owner can give in the chat.

## Questions

When a decision, approval or answer from the owner belongs to a task, ask it as
Questions on the task instead of in chat. If there's no task for it yet,
create one (in linked folders only; elsewhere ask in chat). Anything waiting
on the owner (a
decision, an approval, a step only they can do) is an open Question: a task
message alone never puts a task in their Needs you. Questions never change a
task's status.

```sh
{{cli}} task ask <id> "Question?" --context TEXT --option "Yes" --option "No"
{{cli}} task ask <id> --file questions.json         # 1-5 questions at once
{{cli}} task questions <id> --wait                 # run it in the background
```

- `--file` is a JSON array: `[{"body": "...", "context": "...", "options": ["..."]}]`.
- Each entry is exactly one question the owner can answer on its own, never a
  list and never "see the description". Give short context and up to 4 options
  that are complete answers.
- Wait with `{{cli}} task questions <id> --wait` in the background: it exits
  0 when none are open (`{{cli}} task questions <id> --wait --timeout 30m`
  exits 6 if they're still open). Then post the decisions as a task message
  and act on them.
- Answers can include images: `task questions` saves them and prints their
  paths. Open each one before acting. Additional notes printed under an answer
  apply to the whole set of answers.
- If the owner answers an open question in chat instead, record it in their
  words: `{{cli}} task answer <id> <question-id> "their answer"`
  (`{{cli}} task answer <id> <question-id> "…" --source "in a call"` says
  where; it defaults to "in chat"). Close one that no longer needs an answer with
  `{{cli}} task dismiss <id> <question-id> --reason TEXT`. Question ids come
  from `{{cli}} task questions <id> --status open --json`.

## Messages

- Post progress, results and decisions on the task:
  `{{cli}} task message <id> --body-file PATH`. Bodies are Markdown and come
  only from a file. `{{cli}} task message <id> --body-file PATH --attach PATH`
  adds a file (a screenshot, say).
- When a message answers the owner or is something they asked for (a reply,
  results, screenshots), tag them with their @handle so they're notified;
  `{{cli}} members` lists handles. Messages without a mention don't notify
  anyone. Progress logs don't need a tag.
- Whenever a message (a task message, a chat reply, a question's context, a
  review body) mentions another task, link it:
  `[#373 Voice screen](https://dash.missionbase.app/tasks/373)`. Never a bare
  "#373" or "task 373".
- `{{cli}} task show <id>` prints the whole task as JSON; `{{cli}} task messages <id>`
  prints its thread.

## Task chat

The owner's messages on a task you're running (`task start --terminal`) are
typed into your terminal as:

```text
Missionbase task <id> (…) message: <Name>: "…". Reply: …
```

They're the owner's requests about that task. Reply on the task with
`{{cli}} task message <id> --body-file PATH` (`task messages <id>` for the whole
thread and its attachments), then keep working. Messages route by the terminal
your run records: if you move to another terminal, run
`{{cli}} task start <id> --terminal <new address>` again. Once your run ends
(`task review`, `task stop`, or the work stops), the task's messages go to the
project's orchestrator instead.

## Project chat and watching (orchestrators)

An orchestrator signs its terminal up for the project:
`{{cli}} project watch --terminal $HC_TERMINAL_ID` (one terminal per
project; `{{cli}} project watch --off` stops it). When it's idle, the Herdr bridge types in a digest of
the owner's new tasks, answers, status changes and mentions of you, and the
owner's project chat messages:

```text
Missionbase project <id> (…) chat: <Name>: "…". Reply: …
```

They're the owner's requests. Read the chat and its images with
`{{cli}} project chat show <id>` and answer with
`{{cli}} project chat reply <id> --body-file PATH`. Keep the chat for quick
asks and status: turn anything sizeable into a task and link it in the reply.

Messages on a task nobody is running arrive as `Missionbase task <id> (…)
message, no agent on it (Review): …`. Reply on the task, not in the chat; if it
needs work, move it back to In Progress (or Planning), hand it to an agent and
run `task start <id> --terminal <its address>`.

## Notes and activity

```sh
{{cli}} projects notes show [project-id]           # the project's notes and their version
{{cli}} projects notes append [project-id] --body-file PATH
{{cli}} projects notes edit [project-id] --body-file PATH --base-version VERSION_ID
{{cli}} projects notes history [project-id]
{{cli}} activity --date YYYY-MM-DD --project ID    # what happened on a day
```

- Notes are one Markdown page per project. `append` adds a dated entry under
  `## Log` without rewriting the page; prefer it. `edit` replaces the whole
  page: read it first and pass the version you read as `--base-version`, which
  keeps entries appended since and refuses other changes.
- Without a project id, the project resolves like `tasks ls`.
- `activity` lists the day's events across your owner's projects, oldest
  first (today in the owner's time zone by default).

## Reference

- `{{cli}} tasks ls --needs-you` shows tasks waiting on the owner.
  `{{cli}} tasks ls --status all` (or status keys, `--status todo,planning`)
  widens it; `{{cli}} tasks ls --all` covers every project.
- `{{cli}} task create --title "..." --start-on YYYY-MM-DD` makes a Backlog
  task that moves to To Do that day. `task create` also takes `--deadline`,
  `--recurring`, `--assign-agent` and `--attach`:
  `{{cli}} task create --title "..." --deadline YYYY-MM-DD --recurring weekly --assign-agent SLUG --attach PATH`.
- `{{cli}} task update <id> --title "..." --body-file PATH --deadline YYYY-MM-DD`
  changes a task's title, description and dates.
- `{{cli}} task stop <id>` clears where your work runs and keeps you assigned.
- Project resolution: `--project`, then the nearest `.missionbase/project.json`
  from `--cwd` or here, then the machine default
  (`{{cli}} project --cwd DIR` shows the result).
- Exit codes: 3 = conflict (running elsewhere), 4 = `hc` missing or couldn't
  resolve the terminal, 5 = no project resolved, 6 = `task questions --wait`
  timed out, 1 = any other error.
