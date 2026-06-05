# Spice auth + RBAC demo

Docs:
https://docs.spice.ai/docs/enterprise/features/authentication
https://docs.spice.ai/docs/enterprise/features/policy

Minimal example of how Spice.ai Enterprise gates MCP tool access by API key
and Cedar policy.

## Flow

```
Client (X-API-Key) → Spice /v1/mcp or /v1/tools
                         │
                         ├─ 1. Auth     — who is this caller?
                         └─ 2. Policy   — may they execute this tool?
```

1. **Authentication** (`runtime.auth`) — the `X-API-Key` header identifies the
   caller and maps it to a role (`admin` or `analyst`).

2. **Authorization** (`runtime.authorization`) — on every tool call, Cedar
   evaluates `(principal, action, resource)`:
   - **principal** — the caller's role, e.g. `Spice::Role::"analyst"`
   - **action** — `Spice::Action::"execute"` for tool invocation
   - **resource** — the tool name, e.g. `Spice::Tool::"private_mcp_server/lookup_product"`

   With `default: deny`, anything not explicitly permitted is rejected.

## API keys

| Key               | Role     | Permission |
| ----------------- | -------- | ---------- |
| `admin-api-key`   | `admin`  | ReadWrite  |
| `analyst-api-key` | `analyst`| ReadOnly   |

`ReadOnly` / `ReadWrite` control SQL write paths. Cedar policies control
which datasets and tools each role may use.

## MCP tools

`server.py` exposes five tools through the `private_mcp_server` MCP entry.
Spice registers them as `private_mcp_server/<tool_name>`.

| Tool                    | Analyst | Admin |
| ----------------------- | ------- | ----- |
| `get_sales_summary`     | yes     | yes   |
| `lookup_product`        | yes     | yes   |
| `update_sales_forecast` | no      | yes   |
| `create_product_entry`  | no      | yes   |
| `delete_product_entry`  | no      | yes   |

Analyst permits are listed explicitly in `analyst-mcp-read`. Admin is covered
by the blanket `admins-everything` rule.
