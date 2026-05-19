# Task history queries — `openclaw-mcp-demo`

A set of SQL queries against `runtime.task_history` that are tuned for the
shape of this demo (three routed models, Tool Registry enabled, federated +
accelerated datasets, two MCP servers). Run them through any Spice SQL
interface — REPL, HTTP, Flight SQL.

```bash
spice sql
> SELECT ...
```

> **Note** — `runtime.task_history` is in-memory and retains entries for 8h
> by default. Generate some traffic first (a few `/v1/chat/completions`
> calls or `spice chat` sessions) before running these.

## Schema cheat sheet

```sql
DESCRIBE runtime.task_history;
```

Relevant columns for this demo:

| Column                  | Notes                                                             |
| ----------------------- | ----------------------------------------------------------------- |
| `trace_id`              | One per top-level request (`ai_chat`, `sql_query`, ...).          |
| `span_id`               | One per task within a trace.                                      |
| `parent_span_id`        | Parent within the same `trace_id` — builds the tree.              |
| `task`                  | `ai_chat`, `ai_completion`, `tool_use::<name>`, `sql_query`, ...  |
| `input`, `captured_output` | Prompt / args / response. `captured_output: truncated`.        |
| `execution_duration_ms` | Wall-clock duration of just this span.                            |
| `error_message`         | NULL on success.                                                  |
| `labels`                | `Map<String, String>`. Keys vary by task — see below.             |

Common label keys this demo uses:

| Task                       | Useful label keys                                                    |
| -------------------------- | -------------------------------------------------------------------- |
| `ai_chat`                  | `model`                                                              |
| `ai_completion`            | `model`, `prompt_tokens`, `completion_tokens`, `total_tokens`, `stream` |
| `tool_use::*`              | `tool`                                                               |
| `sql_query`                | `datasets`, `accelerated`, `protocol`, `rows_produced`, `query_execution_duration_ms`, `error_code` |
| `search`                   | `tables`, `limit`                                                    |
| `accelerated_refresh`      | `sql`                                                                |

Read a `labels` value with `labels['model']`. To match against a
comma-separated label value (e.g. `datasets`), use
`ANY(string_to_array(labels['datasets'], ','))`.

---

## 1. Model routing — who served which request?

Which of `chat-router`, `chat-private`, `nsql-coder` got picked, how often,
and how slow.

```sql
SELECT
    labels['model']                    AS model,
    COUNT(*)                           AS requests,
    ROUND(AVG(execution_duration_ms))  AS avg_ms,
    ROUND(MAX(execution_duration_ms))  AS p_max_ms,
    SUM(CASE WHEN error_message IS NOT NULL THEN 1 ELSE 0 END) AS errors
FROM runtime.task_history
WHERE task = 'ai_chat'
  AND start_time >= NOW() - INTERVAL '1 HOUR'
GROUP BY labels['model']
ORDER BY requests DESC;
```

Use this to answer "is traffic actually going where we think it is?"

## 2. Token spend per route

Sums prompt + completion tokens across every `ai_completion` span (a chat
may have several). Useful for cost attribution between the hosted and
private routes.

```sql
SELECT
    labels['model']                                              AS model,
    COUNT(*)                                                     AS completions,
    SUM(CAST(labels['prompt_tokens']     AS BIGINT))             AS prompt_tokens,
    SUM(CAST(labels['completion_tokens'] AS BIGINT))             AS completion_tokens,
    SUM(CAST(labels['total_tokens']      AS BIGINT))             AS total_tokens
FROM runtime.task_history
WHERE task = 'ai_completion'
  AND start_time >= NOW() - INTERVAL '1 HOUR'
  AND labels['model'] IS NOT NULL
GROUP BY labels['model']
ORDER BY total_tokens DESC;
```

## 3. Tool Registry behavior — searches vs. invocations

Because the chat models run `tools: search_registry`, every tool call is
preceded by at least one `tool_search`. This compares the two counts —
ideally each `tool_search` resolves to roughly one `tool_invoke`.

```sql
SELECT
    SUM(CASE WHEN task = 'tool_use::tool_search' THEN 1 ELSE 0 END) AS searches,
    SUM(CASE WHEN task = 'tool_use::tool_invoke' THEN 1 ELSE 0 END) AS invocations,
    ROUND(
        1.0 *
        SUM(CASE WHEN task = 'tool_use::tool_invoke' THEN 1 ELSE 0 END) /
        NULLIF(SUM(CASE WHEN task = 'tool_use::tool_search' THEN 1 ELSE 0 END), 0),
        2
    ) AS invoke_per_search
FROM runtime.task_history
WHERE start_time >= NOW() - INTERVAL '1 HOUR';
```

A ratio well below `1.0` means the model is doing speculative searches that
go nowhere — usually a sign the system prompt's routing hints don't match
the queries users actually ask.

## 4. Which tools is the registry actually surfacing?

Counts the *resolved* tools the model decided to call after a `tool_search`.

```sql
SELECT
    labels['tool']                     AS tool,
    COUNT(*)                           AS invocations,
    ROUND(AVG(execution_duration_ms))  AS avg_ms,
    SUM(CASE WHEN error_message IS NOT NULL THEN 1 ELSE 0 END) AS errors
FROM runtime.task_history
WHERE task LIKE 'tool_use::%'
  AND task <> 'tool_use::tool_search'
  AND task <> 'tool_use::tool_invoke'
  AND start_time >= NOW() - INTERVAL '1 HOUR'
GROUP BY labels['tool']
ORDER BY invocations DESC;
```

Look for `agent_memory/*`, `claw_platform/*`, and built-ins (`sql`,
`search`, `list_datasets`, ...) all showing up — that's the proof point
that the gateway is exposing a unified catalog.

## 5. End-to-end latency split: LLM vs. tool execution

For each top-level `ai_chat`, how much wall time was spent in the model
vs. in tools? Helps diagnose "is the agent slow because the model is slow
or because a tool is slow?"

```sql
WITH per_trace AS (
    SELECT
        t.trace_id,
        MAX(CASE WHEN t.task = 'ai_chat' THEN t.execution_duration_ms END) AS total_ms,
        MAX(CASE WHEN t.task = 'ai_chat' THEN t.labels['model'] END)       AS model,
        SUM(CASE WHEN t.task = 'ai_completion' THEN t.execution_duration_ms ELSE 0 END) AS llm_ms,
        SUM(CASE WHEN t.task LIKE 'tool_use::%' THEN t.execution_duration_ms ELSE 0 END) AS tool_ms
    FROM runtime.task_history t
    WHERE t.start_time >= NOW() - INTERVAL '1 HOUR'
    GROUP BY t.trace_id
    HAVING MAX(CASE WHEN t.task = 'ai_chat' THEN 1 ELSE 0 END) = 1
)
SELECT
    model,
    COUNT(*)                           AS chats,
    ROUND(AVG(total_ms))               AS avg_total_ms,
    ROUND(AVG(llm_ms))                 AS avg_llm_ms,
    ROUND(AVG(tool_ms))                AS avg_tool_ms
FROM per_trace
GROUP BY model
ORDER BY chats DESC;
```

## 6. Federated vs. accelerated query mix

How often did the LLM (via `tool_use::sql`) hit live federated sources
(`crm_accounts`, `support_tickets`) vs. the local accelerated engines
(`sales_daily`, `product_catalog`)?

```sql
SELECT
    CASE
        WHEN labels['accelerated'] = 'true' THEN 'accelerated'
        ELSE 'federated'
    END                                AS mode,
    labels['datasets']                 AS datasets,
    COUNT(*)                           AS queries,
    ROUND(AVG(execution_duration_ms))  AS avg_ms,
    ROUND(AVG(CAST(labels['rows_produced'] AS BIGINT))) AS avg_rows
FROM runtime.task_history
WHERE task = 'sql_query'
  AND start_time >= NOW() - INTERVAL '1 HOUR'
  AND labels['datasets'] IS NOT NULL
GROUP BY mode, labels['datasets']
ORDER BY queries DESC;
```

Acceleration wins should jump out as 10–100× lower `avg_ms` than the
federated rows.

## 7. Did the agent touch a specific dataset?

For "which traces queried `crm_accounts` in the last hour?" — useful when a
customer asks "what did the agent do with my CRM data?"

```sql
SELECT
    trace_id,
    start_time,
    execution_duration_ms,
    SUBSTRING(input, 1, 120) AS sql_preview,
    error_message
FROM runtime.task_history
WHERE task = 'sql_query'
  AND 'crm_accounts' = ANY(string_to_array(labels['datasets'], ','))
  AND start_time >= NOW() - INTERVAL '1 HOUR'
ORDER BY start_time DESC;
```

Pair with `spice trace ai_chat --trace-id <id>` on any result row to see
the surrounding chat.

## 8. NSQL coder — how it's doing

The `/v1/nsql` route lands as `task = 'nsql'`. The completion below it
carries the model label.

```sql
SELECT
    labels['model']                    AS model,
    COUNT(*)                           AS nsql_calls,
    ROUND(AVG(execution_duration_ms))  AS avg_ms,
    SUM(CASE WHEN error_message IS NOT NULL THEN 1 ELSE 0 END) AS errors
FROM runtime.task_history
WHERE task = 'nsql'
  AND start_time >= NOW() - INTERVAL '1 HOUR'
GROUP BY labels['model']
ORDER BY nsql_calls DESC;
```

For per-call SQL output (was the generated query actually valid?):

```sql
SELECT
    start_time,
    execution_duration_ms,
    SUBSTRING(input,           1, 100) AS nl_question,
    SUBSTRING(captured_output, 1, 200) AS generated_sql,
    error_message
FROM runtime.task_history
WHERE task = 'nsql'
ORDER BY start_time DESC
LIMIT 20;
```

## 9. Recent failures, grouped by where they happened

```sql
SELECT
    task,
    COUNT(*) AS failures,
    SUBSTRING(MIN(error_message), 1, 120) AS sample_error,
    MAX(start_time) AS last_seen
FROM runtime.task_history
WHERE error_message IS NOT NULL
  AND start_time >= NOW() - INTERVAL '1 HOUR'
GROUP BY task
ORDER BY failures DESC;
```

To trace a specific failure back to the originating chat:

```sql
SELECT trace_id
FROM runtime.task_history
WHERE error_message IS NOT NULL
ORDER BY start_time DESC
LIMIT 1;

-- then:
-- spice trace ai_chat --trace-id <that trace_id> --include-input
```

## 10. Acceleration refresh health

Are the two accelerated datasets refreshing on schedule, and how long do
the refreshes take?

```sql
SELECT
    labels['sql']                      AS refresh_sql,
    COUNT(*)                           AS refreshes,
    ROUND(AVG(execution_duration_ms))  AS avg_ms,
    ROUND(MAX(execution_duration_ms))  AS max_ms,
    MAX(start_time)                    AS last_refresh,
    SUM(CASE WHEN error_message IS NOT NULL THEN 1 ELSE 0 END) AS errors
FROM runtime.task_history
WHERE task = 'accelerated_refresh'
  AND start_time >= NOW() - INTERVAL '6 HOURS'
GROUP BY labels['sql']
ORDER BY last_refresh DESC;
```

## 11. Full chain of the most recent chat (mirrors `spice trace`)

Useful when you want the trace as a table you can join against rather than
a CLI tree.

```sql
SELECT
    task,
    span_id,
    parent_span_id,
    execution_duration_ms,
    labels['model']   AS model,
    labels['tool']    AS tool,
    SUBSTRING(input,           1, 80) AS input_preview,
    SUBSTRING(captured_output, 1, 80) AS output_preview,
    error_message
FROM runtime.task_history
WHERE trace_id = (
    SELECT trace_id
    FROM runtime.task_history
    WHERE task = 'ai_chat'
    ORDER BY start_time DESC
    LIMIT 1
)
ORDER BY start_time;
```
