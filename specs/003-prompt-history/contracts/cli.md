# CLI Contract

## Launch

```text
goagent [--workspace PATH] [--provider NAME] [--config PATH]
```

Launch flags and startup behavior remain unchanged.

## Session Input

- Ordinary lines are treated as natural-language task input for the agent.
- Slash commands are handled locally by the CLI and are not sent to the model.
- In interactive TTY mode, submitted non-empty lines are recorded in
  session-local prompt history after the user presses Enter.
- In non-interactive input mode, line reading remains buffered and does not
  provide arrow-key prompt history.

## Interactive Keyboard Behavior

- `Up Arrow`: Recall the previous non-empty submitted line from the current
  interactive session, newest first.
- `Down Arrow`: Move toward newer recalled lines; from the newest recalled line,
  restore the draft that was visible before history navigation began.
- Repeated `Up Arrow` at the oldest saved entry is a no-op and keeps that entry
  visible.
- Repeated `Down Arrow` after the live draft has been restored is a no-op and
  leaves the visible input unchanged.
- Pressing `Up Arrow` or `Down Arrow` with no saved history is a no-op.
- Recalled lines stay editable before submission, and submitting an edited
  recalled line appends a new history entry without changing the earlier saved
  entry.

## Slash Commands

```text
/approve write|web|module
/attach PATH
/detach NAME
/provider NAME
/tools
/quit
```

Recalled slash-command lines use the same local suggestion rendering as freshly
typed slash input and still execute only when the user presses Enter.

## Session Output

- Assistant text is streamed to stdout as provider chunks arrive.
- Tool execution summaries and approval prompts are printed as structured CLI
  lines.
- Interactive prompt redraw continues to own prompt text and slash suggestions.

## Error Cases

- Unknown slash command: print usage and continue.
- Unsupported provider override: print the configured provider names and
  continue without changing provider.
- Missing approval for a risky non-shell action: print the required `/approve`
  command and skip the action.
- Unsupported escape sequences in interactive mode: ignore them without
  corrupting visible input or submitting unintended text.
- Out-of-workspace path or command request: reject the action and explain the
  boundary.
