# Quickstart: Slash Command Discovery and Per-Command Approval

## 1. Start an Interactive Session

```bash
goagent --workspace /path/to/repo
```

Expected behavior:

- Starts one fresh interactive session.
- Uses the existing built-in tools and provider configuration.
- Keeps write/web/module approvals separate from shell approvals.

## 2. Inspect Slash Commands In Terminal

At the prompt, type:

```text
/
```

Expected behavior:

- The terminal shows the available slash commands.
- Each entry includes the command usage plus a short description.
- Narrowing the slash prefix (for example `/to`) filters the visible suggestions.

## 3. Run a Shell Task That Needs Approval

Example prompt:

```text
Run `pwd` and explain the result.
```

Expected behavior:

- When the agent chooses the shell tool, the CLI asks for approval before
  executing `pwd`.
- Reply `yes` or `y` to approve.
- The approved shell command runs immediately without re-entering the prompt.

## 4. Verify Exact-Command Reuse

Prompt again:

```text
Run `pwd` again.
```

Expected behavior:

- The same exact command in the same working directory does not prompt again in
  the same session.

## 5. Verify Argument Changes Need Fresh Approval

Prompt:

```text
Run `pwd -P` and explain the difference.
```

Expected behavior:

- The CLI asks for approval again because the command text changed.
- Denying the prompt skips execution and leaves the session running.

## 6. Use Non-Shell Approvals Separately

If the agent needs to write a file or use the web tool, approve those
capabilities explicitly:

```text
/approve write
/approve web
```

Expected behavior:

- Non-shell approvals remain session-scoped capability approvals.
- Shell commands continue to use inline exact-command approval prompts.
