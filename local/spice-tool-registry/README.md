# Spice MCP Tools registry example

Showcase Spice MCP tools registry

Docs:
- https://spiceai.org/docs/next/features/large-language-models/mcp
- https://spiceai.org/docs/next/components/tools/mcp

Start runtime:

```
spice run
```

List loaded tools:

```
curl -H 'x-api-key: foobar' http://127.0.0.1:8090/v1/tools | jq '.[].name'
  % Total    % Received % Xferd  Average Speed  Time    Time    Time   Current
                                 Dload  Upload  Total   Spent   Left   Speed
100  12056 100  12056   0      0  2.81M      0                              0
"sample_distinct_columns"
"get_current_datetime"
"top_n_sample"
"get_readiness"
"random_sample"
"store_memory"
"list_datasets"
"sql"
"table_schema"
"mcp_server_a/get_sales_summary"
"mcp_server_a/lookup_product"
"mcp_server_a/update_sales_forecast"
"mcp_server_a/create_product_entry"
"mcp_server_a/delete_product_entry"
"mcp_server_b/get_sales_summary"
"mcp_server_b/lookup_product"
"mcp_server_b/update_sales_forecast"
"mcp_server_b/create_product_entry"
"mcp_server_b/delete_product_entry"
"load_memory"
"search"
```

Note that while tools have same names (since coming from the same MCP server), all prefixed with corresponding name in spicepod `mcp_server_a` and `mcp_server_b`.
