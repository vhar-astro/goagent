# Quickstart: Interactive Prompt History Recall

## 1. Run the test suite

```bash
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/go-mod /opt/go/go/bin/go test ./...
```

Expected behavior:

- Existing CLI, session, and integration tests stay green.
- New `internal/cli` history-navigation tests cover boundary behavior and draft
  restoration.

## 2. Start an interactive session

```bash
goagent --workspace /path/to/repo
```

Use a configured provider environment only if you want to validate ordinary
prompt submission. Slash-command recall can be validated locally.

## 3. Validate prompt recall

Enter several submitted lines, for example:

```text
Explain the current workspace layout.
/tools
Summarize the last tool output.
```

Then verify:

- `Up Arrow` recalls `Summarize the last tool output.` first.
- `Up Arrow` again recalls `/tools`.
- `Up Arrow` again recalls `Explain the current workspace layout.`.
- `Up Arrow` at the oldest entry keeps `Explain the current workspace layout.`
  visible.

## 4. Validate draft preservation

Type an unsent draft such as:

```text
Draft that should come back
```

Before pressing Enter:

- Press `Up Arrow` to enter history.
- Press `Down Arrow` once to move toward the newest saved entry.
- Press `Down Arrow` again to move past the newest saved entry.
- Confirm the original unsent draft is restored exactly.

## 5. Validate edit and resubmit

- Recall an earlier line with `Up Arrow`.
- Edit part of the visible text.
- Press Enter.
- Recall history again and confirm both the original entry and the edited
  resubmission appear as separate entries in history.

## 6. Validate recalled slash commands

- Submit `/tools`.
- Press `Up Arrow` to recall `/tools`.
- Confirm the `/tools` suggestion renders locally while the recalled command is
  visible.
- Press Enter and confirm `/tools` executes again.

## 7. Validate non-interactive fallback

```bash
printf '/tools\n' | goagent --workspace /path/to/repo
```

Expected behavior:

- The command still runs through the existing line-buffered path.
- Arrow-key history recall is interactive-only and is not available in piped
  input mode.
