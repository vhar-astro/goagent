# Module Protocol Contract

## Module Layout

Each attachable module is a local directory containing:

```text
module.json
<entrypoint executable>
```

## Manifest Schema

```json
{
  "name": "string",
  "version": "string",
  "requested_capabilities": ["write", "shell"],
  "entrypoint": "./bin/module",
  "tools": [
    {
      "name": "tool_name",
      "description": "what the tool does",
      "input_schema": {
        "type": "object"
      },
      "capability": "module"
    }
  ]
}
```

## Validation Rules

- `name`, `version`, `requested_capabilities`, `entrypoint`, and `tools` are
  required.
- `entrypoint` must resolve within the module directory and be executable.
- Tool names must not collide with built-in tools or other attached modules.
- Requested capabilities must be approved before the module becomes active.

## Stdio Transport

- Transport is newline-delimited JSON over stdin/stdout.
- The agent writes one request object per line.
- The module writes one response object per line.
- Any non-JSON line is treated as a protocol error.

## Message Types

### Init Request

```json
{
  "type": "init",
  "session_id": "session-123",
  "workspace_root": "/abs/workspace/root"
}
```

### Init Response

```json
{
  "type": "ready",
  "module": "example-module",
  "version": "1.0.0"
}
```

### Tool Call Request

```json
{
  "type": "call",
  "id": "call-123",
  "tool": "tool_name",
  "arguments": {
    "key": "value"
  }
}
```

### Success Response

```json
{
  "type": "result",
  "id": "call-123",
  "content": "tool result text"
}
```

### Error Response

```json
{
  "type": "error",
  "id": "call-123",
  "message": "what failed"
}
```

## Lifecycle

- `attach`: validate manifest, ensure approvals, start process, send `init`,
  require `ready`.
- `call`: route tool invocations by tool name to the owning module process.
- `detach`: stop the module process and remove its tools from the active tool
  registry.
