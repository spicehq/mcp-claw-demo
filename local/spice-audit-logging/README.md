# Spice Audit Logging via task_history

Showcase Spice MCP tools registry

Docs:
- https://spiceai.org/docs/reference/task_history
- https://spiceai.org/docs/reference/spicepod/runtime#runtimetask_history

Start runtime:

```
spice run
```

Perform some actions:

run sql query

```
spice sql --api-key foobar --query 'show tables;'
```

list registered tools

```
curl -H 'x-api-key: foobar' http://127.0.0.1:8090/v1/tools | jq '.[].name'
  % Total    % Received % Xferd  Average Speed  Time    Time    Time   Current
                                 Dload  Upload  Total   Spent   Left   Speed
100  10649 100  10649   0      0  1.99M      0                              0
"load_memory"
"table_schema"
"get_current_datetime"
"server/get_sales_summary"
"server/lookup_product"
"server/update_sales_forecast"
"server/create_product_entry"
"server/delete_product_entry"
"store_memory"
"get_readiness"
"random_sample"
"sample_distinct_columns"
"top_n_sample"
"search"
"list_datasets"
"sql"
```

call tools

```
curl -H 'x-api-key: foobar' -X POST http://127.0.0.1:8090/v1/tools/get_readiness
{"dataset:runtime.task_history":"Ready","tool:store_memory":"Ready","tool_catalog:memory":"Ready","tool:random_sample":"Ready","tool:list_datasets":"Ready","tool:table_schema":"Ready","tool:top_n_sample":"Ready","tool:sql":"Ready","tool:load_memory":"Ready","tool:get_readiness":"Ready","tool:search":"Ready","tool_catalog:auto":"Ready","tool:get_current_datetime":"Ready","tool:sample_distinct_columns":"Ready","tool:server":"Ready"}
```

```
curl -H 'x-api-key: foobar' -X POST http://127.0.0.1:8090/v1/tools/server/update_sales_forecast
[{"type":"text","text":"3 validation errors for call[update_sales_forecast]\nsku\n  Missing required argument [type=missing_argument, input_value={}, input_type=dict]\n    For further information visit https://errors.pydantic.dev/2.13/v/missing_argument\nregion\n  Missing required argument [type=missing_argument, input_value={}, input_type=dict]\n    For further information visit https://errors.pydantic.dev/2.13/v/missing_argument\nunits\n  Missing required argument [type=missing_argument, input_value={}, input_type=dict]\n    For further information visit https://errors.pydantic.dev/2.13/v/missing_argument"}]
```

```
curl -H 'x-api-key: foobar' -X POST http://127.0.0.1:8090/v1/tools/server/delete_product_entry -d '{"sku": "123"}'
[{"type":"text","text":"[demo] Deleted product 123 from catalog (write applied)"}]
```

check task_history:

```
spice sql
```


```
sql> select trace_id, task, input, start_time, labels from runtime.task_history;
+----------------------------------+----------------------------------------+--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------+--------------------------------+---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------+
|             trace_id             |                  task                  |                                                                                                                                                     input                                                                                                                                                    |           start_time           |                                                                                      labels                                                                                     |
|              varchar             |                 varchar                |                                                                                                                                                    varchar                                                                                                                                                   |       timestamp[ns] (UTC)      |                                                                              map<varchar, varchar>                                                                              |
+----------------------------------+----------------------------------------+--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------+--------------------------------+---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------+
| a387c80522ecdc69b8730b52697ed260 | acceleration_refresh                   | runtime.task_history                                                                                                                                                                                                                                                                                         | 2026-06-05T13:17:45.919938058Z | {sql: SELECT trace_id, span_id, parent_span_id, "task", "input", captured_output, start_time, end_time, execution_duration_ms, error_message, labels FROM runtime.task_history} |
| 95fcc7efb3277fcb24f5c173de06480d | sql_query                              | show tables;                                                                                                                                                                                                                                                                                                 | 2026-06-05T13:18:19.884773158Z | {query_execution_duration_ms: 0.825306, rows_produced: 8, protocol: FlightSQL, datasets: information_schema.tables}                                                             |
| e7623249f515f8d5b831cbd8ea301ef5 | tool_use::sql                          |                                                                                                                                                                                                                                                                                                              | 2026-06-05T13:20:30.126210291Z | {tool: sql}                                                                                                                                                                     |
| 83d4643b5389f2f7074d6cf00d44a520 | tool_use::get_readiness                |                                                                                                                                                                                                                                                                                                              | 2026-06-05T13:20:46.932730252Z | {tool: get_readiness}                                                                                                                                                           |
| ad854330457d16192314121071fbab4c | tool_use::server/update_sales_forecast |                                                                                                                                                                                                                                                                                                              | 2026-06-05T13:21:28.924848768Z | {tool: update_sales_forecast, task_override: tool_use::server/update_sales_forecast, mcp_server: server}                                                                        |
| a412ec16b6a88aa7a64477dc6b7101fd | tool_use::server/delete_product_entry  | {"sku": "123"}                                                                                                                                                                                                                                                                                               | 2026-06-05T13:22:44.731848177Z | {tool: delete_product_entry, mcp_server: server, task_override: tool_use::server/delete_product_entry}                                                                          |
```
