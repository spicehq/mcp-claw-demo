# OpenClaw + Spice — MCP gateway & model routing demo

A self-contained Spicepod that shows how Spice can serve as the **single pane
of glass** behind a personal-agent platform like OpenClaw / NemoClaw:

- **Federated + accelerated data** exposed as one SQL surface.
- **MCP gateway** — a local stdio MCP and a remote HTTP MCP are re-exposed,
  alongside Spice's built-in tools, over `/v1/mcp`.
- **Model routing with the Tool Registry** — three routed models (hosted,
  private, NSQL) share the same internal tool catalog. The chat models see
  only `tool_search` and `tool_invoke` meta-tools per turn, keeping
  per-request tokens flat as the catalog grows.

The same spicepod supports two integration patterns. Your agent platform
can pick either — or use both, the same Spice instance answers both.

---

## Pattern A — Pure MCP gateway (BYO model)

Your agent platform keeps its own LLM provider (Anthropic, hosted OpenAI,
self-hosted — whichever). Spice is wired in as an MCP server. Every tool —
built-in (`sql`, `search`, `list_datasets`, …), local stdio
(`agent_memory/*`), and remote HTTP (`claw_platform/*`) — appears in one
unified catalog at `/v1/mcp`.

```
              ┌──────────────────────────────────────────────────────────┐
              │                          SPICE                           │
              │                                                          │
              │   /v1/mcp  ── MCP server (Streamable HTTP) ──────┐       │
              │                                                  │       │
  ┌────────┐  │   built-in tools                                 │       │
  │OpenClaw│  │     sql, list_datasets, table_schema,            │       │
  │  +     │◄─┤     search, sample_distinct_columns, memory      │       │
  │ Claude │  │                                                  │       │
  │  (any  │  │   agent_memory/*  ── stdio ──► npx @mcp/memory   │       │
  │  LLM)  │  │                                                  │       │
  └────────┘  │   claw_platform/* ── HTTP  ──► remote MCP        │       │
              │                                                  │       │
              │   datasets backing `sql`:                        │       │
              │     crm_accounts    ── federated ──► Postgres    │       │
              │     support_tickets ── federated ──► GitHub      │       │
              │     sales_daily     ── accelerated (Arrow)       │       │
              │     product_catalog ── accelerated (SQLite)      │       │
              └──────────────────────────────────────────────────────────┘
```

What this gives you:

- One MCP endpoint to integrate against, regardless of which underlying
  data system or MCP server answers each call.
- Federation + caching out of the box — `sql` against `sales_daily` is a
  local Arrow scan; `sql` against `crm_accounts` hits the live Postgres
  replica.
- A single trace ID for every cross-system call, queryable as SQL.

---

## Pattern B — Full Spice (model routing + internal tool loop)

Point your OpenAI-compatible client at Spice's `/v1/chat/completions` and
pick a model by name. Spice runs the tool loop internally against the same
catalog Pattern A exposes.

```
              ┌──────────────────────────────────────────────────────────┐
              │                          SPICE                           │
              │                                                          │
              │   /v1/chat/completions   /v1/nsql                        │
              │           │                  │                           │
  ┌────────┐  │           ▼                  ▼                           │
  │OpenClaw│──┤    model router ──► chat-router   (OpenAI gpt-5.4)       │
  │ (any   │  │                  ├► chat-private  (private endpoint)     │
  │ OpenAI │  │                  └► nsql-coder    (gpt-5.4-mini + prompt)│
  │ client)│  │                                                          │
  └────────┘  │   Chat models see only TWO meta-tools per turn:          │
              │     tool_search(query, keywords?, limit?)                │
              │     tool_invoke(tool_id, arguments)                      │
              │                  │                                       │
              │                  ▼                                       │
              │       ┌─── Tool Registry (hybrid: full-text +            │
              │       │      keyword + schema + vector via text_embed)   │
              │       ▼                                                  │
              │     resolves to one of:                                  │
              │       sql, search, list_datasets, table_schema, ...      │
              │       agent_memory/*  (stdio)                            │
              │       claw_platform/* (HTTP)                             │
              │                  │                                       │
              │                  ▼                                       │
              │       Postgres │ GitHub │ S3 (Arrow) │ S3 (SQLite)       │
              └──────────────────────────────────────────────────────────┘
```

What this gives you:

- One HTTP endpoint instead of orchestrating model and tools client-side.
- Per-request model choice — `chat-private` for sensitive data,
  `nsql-coder` for analytics, `chat-router` for everything else. The tool
  surface is identical across all three.
- **Bounded per-turn token cost.** The Tool Registry means every chat turn
  carries only two meta-tool schemas, no matter how big the catalog grows.
- One trace ID covers every model hop, every `tool_search`, and every
  resolved tool call.

---

## What's in this Spicepod

### Datasets

| Name              | Source                            | Mode        | Why                                              |
| ----------------- | --------------------------------- | ----------- | ------------------------------------------------ |
| `crm_accounts`    | Postgres `public.accounts`        | Federated   | CRM truth must be live, no ETL.                  |
| `support_tickets` | GitHub Issues (stand-in)          | Federated   | Tickets churn constantly.                        |
| `sales_daily`     | S3 parquet (`cleaned_sales_data`) | Accelerated | Hot analytics — Arrow in-memory, refresh hourly. |
| `product_catalog` | S3 parquet (taxi_trips stand-in)  | Accelerated | Reference data — SQLite on disk, refresh 6h.     |

### Tools

| Name            | `from:`                                              | Transport       | What it adds                                     |
| --------------- | ---------------------------------------------------- | --------------- | ------------------------------------------------ |
| `agent_memory`  | `mcp:npx` (`@modelcontextprotocol/server-memory`)    | stdio, local    | Persistent knowledge graph for the agent.        |
| `claw_platform` | `mcp:http://...`                                     | Streamable HTTP | Proxy to an internal platform MCP.               |

Built-in tools (always on): `sql`, `list_datasets`, `table_schema`,
`sample_distinct_columns`, `search`, `top_n_sample`, `random_sample`,
`load_memory`, `store_memory`, `get_readiness`.

### Models

| Name           | `from:`                          | `tools:`          | Notes                                                                                  |
| -------------- | -------------------------------- | ----------------- | -------------------------------------------------------------------------------------- |
| `chat-router`  | `openai:gpt-5.4`                 | `search_registry` | General orchestrator. Sees only `tool_search` + `tool_invoke`.                          |
| `chat-private` | `openai:acme-llama-3.1-70b-...`  | `search_registry` | Same shape, pointed at your private OpenAI-compatible endpoint.                         |
| `nsql-coder`   | `openai:gpt-5.4-mini`            | explicit 4-tool   | Natural-language-to-SQL with a tight schema-aware system prompt.                        |

### Embeddings

| Name         | `from:`                          | Used for                                                              |
| ------------ | -------------------------------- | --------------------------------------------------------------------- |
| `text_embed` | `openai:text-embedding-3-small`  | Tool Registry vector channel (required by `tools: search_registry`).  |

---

## Run it locally

The repo ships a docker compose stack for everything that needs
infrastructure (Postgres CRM, a stub "private" LLM, and the second Spice
instance that backs `claw_platform`). The main Spice runs on your host.

### Prerequisites

- macOS or Linux, with `docker`, `jq`, `curl`, and `node` (for the
  `agent_memory` MCP via `npx`).
- An **OpenAI API key** — used by `chat-router`, `nsql-coder`, and
  `text_embed`.
- A **GitHub PAT** with `repo` scope for the `support_tickets` dataset, and
  `workflow` scope if you install Spice from nightly (next step).

### 1. Install Spice (nightly)

The demo uses the Tool Registry, which is available on the nightly build.

```bash
curl -fsSL https://raw.githubusercontent.com/spiceai/spiceai/trunk/install/install-nightly.sh \
  -o /tmp/install-spice-nightly.sh

GITHUB_TOKEN=ghp_xxx \
SPICED_INSTALL_DIR="$HOME/.spice/bin" \
bash /tmp/install-spice-nightly.sh
```

`SPICED_INSTALL_DIR="$HOME/.spice/bin"` ensures `spice run` picks up the
nightly `spiced`.

```bash
spice version
# Runtime version: v2.0.0-unstable.nightly....
```

(If you don't need the Tool Registry, `spice install v2.0.0-rc.4` works
too — the demo runs, the chat models just fall back to exposing every tool
directly.)

### 2. Configure `.env`

```bash
cp .env.example .env
# Fill in OPENAI_API_KEY and GITHUB_TOKEN. The remaining values already
# point at the local stack.
```

### 3. Start the support services

```bash
docker compose up -d
```

| Service               | Port     | What it is                                                                  |
| --------------------- | -------- | --------------------------------------------------------------------------- |
| `postgres`            | `:5432`  | Seeded with `public.accounts` (12 rows) for `crm_accounts`.                 |
| `stub-llm`            | `:8081`  | OpenAI-compatible stub that serves `chat-private` with a canned response.   |
| `claw-platform`       | internal | A second Spice instance hosting `deploys` + `runbooks` as the proxy target. |
| `claw-platform-proxy` | `:8092`  | nginx that injects `X-API-Key` for the main Spice's MCP client.\*           |

\* Temporary — Spice's MCP client will soon support the auth key directly
on the tool, at which point this sidecar can be removed.

### 4. Start Spice

From the project directory:

```bash
spice run
```

You should see `All components are loaded. Spice runtime is ready!` within
about 15 seconds.

---

## Try it

All API calls require `X-API-Key: openclaw-demo-key` because the spicepod
enables `runtime.auth.api-key` (v2 requires auth on `/v1/mcp`).

### Pattern A — MCP gateway

```bash
K=openclaw-demo-key

# Unified tool catalog: built-ins + agent_memory/* + claw_platform/*.
curl -s -H "X-API-Key: $K" http://127.0.0.1:8090/v1/tools \
  | jq -r '.[].name' | sort

# SQL across the federated CRM.
curl -s -X POST -H "X-API-Key: $K" -H "Content-Type: application/json" \
  http://127.0.0.1:8090/v1/tools/sql \
  -d '{"query":"SELECT plan, COUNT(*) AS n FROM crm_accounts GROUP BY plan ORDER BY n DESC"}'
# → [{"plan":"Enterprise","n":6},{"plan":"Pro","n":4},{"plan":"Starter","n":2}]

# SQL across an accelerated S3 → Arrow dataset.
curl -s -X POST -H "X-API-Key: $K" -H "Content-Type: application/json" \
  http://127.0.0.1:8090/v1/tools/sql \
  -d '{"query":"SELECT COUNT(*) AS rows FROM sales_daily"}'

# Proxied call — main Spice → claw_platform → deploys.
curl -s -X POST -H "X-API-Key: $K" -H "Content-Type: application/json" \
  http://127.0.0.1:8090/v1/tools/claw_platform/sql \
  -d '{"query":"SELECT service, status, COUNT(*) AS n FROM deploys GROUP BY service, status ORDER BY n DESC LIMIT 5"}'

# Stdio MCP — Spice supervises the npx process.
curl -s -X POST -H "X-API-Key: $K" -H "Content-Type: application/json" \
  http://127.0.0.1:8090/v1/tools/agent_memory/read_graph -d '{}'
```

Any MCP client speaking Streamable HTTP can attach at
`http://127.0.0.1:8090/v1/mcp` with header `X-API-Key: openclaw-demo-key`.

### Pattern B — model routing

```bash
K=openclaw-demo-key

# The three models on offer.
curl -s -H "X-API-Key: $K" http://127.0.0.1:8090/v1/models \
  | jq -r '.data[].id'

# Private route — `chat-private` hits the stub LLM.
curl -s -X POST -H "X-API-Key: $K" -H "Content-Type: application/json" \
  http://127.0.0.1:8090/v1/chat/completions \
  -d '{"model":"chat-private","messages":[{"role":"user","content":"hi"}]}' \
  | jq -r '.choices[0].message.content'

# Hosted route — `chat-router` hits real OpenAI.
curl -s -X POST -H "X-API-Key: $K" -H "Content-Type: application/json" \
  http://127.0.0.1:8090/v1/chat/completions \
  -d '{"model":"chat-router","messages":[{"role":"user","content":"How many accounts are on the Enterprise plan? Use the sql tool."}]}' \
  | jq -r '.choices[0].message.content'
# → "There are 6 accounts on the Enterprise plan."

# Natural language to SQL.
curl -s -X POST -H "X-API-Key: $K" -H "Content-Type: application/json" \
  http://127.0.0.1:8090/v1/nsql \
  -d '{"model":"nsql-coder","query":"top 3 accounts by mrr"}'
# → [{"name":"Stark Industries","mrr":64000.00}, ...]
```

---

## Observability

Every call into Spice — model hop, tool selection, MCP invocation, SQL
query against any dataset — lands in `runtime.task_history` with a single
trace ID. The spicepod has `captured_output: truncated`, so inputs and
(truncated) outputs are available alongside timings.

Two ways to use it:

- **`spice trace`** — CLI for inspecting a single trace.
- **SQL over `runtime.task_history`** — analytics across many traces. See
  **[`task-history-queries.md`](./task-history-queries.md)** for a set of
  ready-to-run queries.

### `spice trace`

```bash
spice trace ai_chat   --api-key openclaw-demo-key
spice trace nsql      --api-key openclaw-demo-key
spice trace sql_query --api-key openclaw-demo-key
```

Useful flags:

| Flag                | What it does                                                                |
| ------------------- | --------------------------------------------------------------------------- |
| `--trace-id <id>`   | Pin to one trace (the `trace_id` column from `runtime.task_history`).       |
| `--id <id>`         | Look up by `labels['id']` (e.g. an OpenAI `chatcmpl-...` completion id).    |
| `--include-input`   | Add the prompt / SQL / tool args as a column.                               |
| `--include-output`  | Add the captured (truncated) response as a column.                          |

A `chat-router` trace from the agentic call above, with the Tool Registry
on:

```
 Tree                     Status  Duration    Span ID
 ai_chat                  OK       4623.04ms  b6e79e16c0470d84
 tool_use::list_datasets  OK          0.02ms  341f4a5afee9fcae
 ai_completion            OK       1598.82ms  8b8f12d4a3e64193
 tool_use::tool_search    OK        542.91ms  ddad90174d14f84e
 text_embed               OK        131.16ms  f76a80012d54fde8
 text_embed               OK        411.23ms  a726671c74230826
 ai_completion            OK       1276.46ms  19f38f1dda5425b2
 tool_use::tool_invoke    OK          1.79ms  6a78ad163da08a50
 tool_use::sql            OK          1.77ms  bdd4667ee35159e7
 sql_query                OK          1.73ms  a4479e48a3267b69
 ai_completion            OK       1202.31ms  034e1b32991c08aa
```

Three things this surfaces in one shot:

- **Model routing** — `ai_chat` carries `labels['model']`, so you can tell
  which model served the request.
- **Tool Registry behavior** — `tool_use::tool_search` and the resolved
  `tool_use::tool_invoke` show up as separate spans, so you can see which
  tool the registry surfaced and how long the lookup took.
- **Data path** — the leaf `sql_query` carries `labels['datasets']` and
  `labels['accelerated']`, so a federated Postgres scan is visibly
  different from an accelerated Arrow scan.

### SQL

For analytics across many traces — token spend per model, registry hit
rate, federated vs. accelerated split, NSQL latency — see
**[`task-history-queries.md`](./task-history-queries.md)**.

Two quick examples:

```bash
K=openclaw-demo-key

# Who served which request?
curl -s -X POST -H "X-API-Key: $K" -H "Content-Type: application/json" \
  http://127.0.0.1:8090/v1/sql -d '{"sql":"SELECT labels['"'"'model'"'"'] AS model, COUNT(*) AS requests, ROUND(AVG(execution_duration_ms)) AS avg_ms FROM runtime.task_history WHERE task = '"'"'ai_chat'"'"' GROUP BY labels['"'"'model'"'"'] ORDER BY requests DESC","parameters":[]}'
# → [{"model":"chat-router","requests":2,"avg_ms":2783.0},
#    {"model":"chat-private","requests":1,"avg_ms":4.0}]

# Tool Registry health: tool_search vs tool_invoke counts.
curl -s -X POST -H "X-API-Key: $K" -H "Content-Type: application/json" \
  http://127.0.0.1:8090/v1/sql -d '{"sql":"SELECT SUM(CASE WHEN task = '"'"'tool_use::tool_search'"'"' THEN 1 ELSE 0 END) AS searches, SUM(CASE WHEN task = '"'"'tool_use::tool_invoke'"'"' THEN 1 ELSE 0 END) AS invocations FROM runtime.task_history","parameters":[]}'
# → [{"searches":1,"invocations":1}]
```

Or use the REPL: `spice sql --api-key openclaw-demo-key`.

### Persisting beyond the retention window

`runtime.task_history` is in-memory with an 8h default retention. For
longer horizons (cost reporting, weekly reviews) the standard pattern is a
worker that periodically appends to a durable sink — Iceberg, Postgres,
S3 — see [Persisting Task History](https://docs.spiceai.org/reference/task_history#persisting-task-history).
