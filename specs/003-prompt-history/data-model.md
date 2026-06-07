# Data Model: Interactive Prompt History Recall

## PromptHistoryEntry

| Field | Type | Description |
|---|---|---|
| `line` | string | Raw submitted input line exactly as entered before parsing |
| `sequence` | int | Submission order within the current interactive session |
| `kind` | string | `prompt` or `slash`, derived from the raw line at dispatch time |

**Validation**:

- `line` must be non-empty after trimming for submission.
- Duplicate `line` values are allowed and remain separate entries.
- Entries exist only for the lifetime of the current process.

## DraftInput

| Field | Type | Description |
|---|---|---|
| `text` | string | Current unsent visible input line |
| `source` | string | `draft` or `history` depending on how the visible text was loaded |
| `preserved_before_history` | string | The draft captured when history navigation first began |

**Validation**:

- `preserved_before_history` is captured only once when moving from the live
  draft into history navigation.
- Editing visible recalled text changes only `text`; it does not mutate stored
  history entries.

## HistoryCursor

| Field | Type | Description |
|---|---|---|
| `position` | int | Index into history entries, or the live-draft sentinel at `len(entries)` |
| `navigating` | bool | Whether the visible buffer is currently showing a history-derived state |

**State transitions**:

- `live draft -> navigating older entry` on up-arrow when history exists.
- `older entry -> newer entry` on down-arrow while `position < len(entries)-1`.
- `newest entry -> live draft` on down-arrow at the newest history boundary.
- Oldest and newest boundaries clamp; repeated arrows do not move beyond them.

## PromptHistoryBuffer

| Field | Type | Description |
|---|---|---|
| `entries` | []PromptHistoryEntry | Session-local ordered history of submitted raw lines |
| `cursor` | HistoryCursor | Current navigation position |
| `visible` | DraftInput | The line currently shown in the prompt area |

**Validation**:

- `entries` are append-only during a session.
- The cursor must stay within `0..len(entries)` when entries exist.
- When `entries` are empty, up/down navigation is a no-op and the visible draft
  remains unchanged.

## Relationships

- One interactive `REPL` owns one `PromptHistoryBuffer`.
- One `PromptHistoryBuffer` has many `PromptHistoryEntry` items in submission
  order.
- One `PromptHistoryBuffer` has one active `HistoryCursor` and one visible
  `DraftInput`.
