"""Demo MCP server for Spice RBAC — read tools for analysts, write tools for admins."""

from fastmcp import FastMCP

mcp = FastMCP("AcmeClaw RBAC Demo")


# ---- Read tools (analyst + admin) -------------------------------------------


@mcp.tool
def get_sales_summary(region: str, days: int = 7) -> str:
    """Return a dummy daily sales rollup summary for a region."""
    return (
        f"[demo] Sales summary for {region} over the last {days} days: "
        "revenue=$1,284,500, units=4,218, top_sku=WIDGET-A"
    )


@mcp.tool
def lookup_product(sku: str) -> str:
    """Look up product catalog details by SKU (read-only)."""
    return (
        f"[demo] Product {sku}: name=Acme Widget, category=hardware, "
        "list_price=49.99, status=active"
    )


# ---- Write tools (admin only) -----------------------------------------------


@mcp.tool
def update_sales_forecast(sku: str, region: str, units: int) -> str:
    """Update the sales forecast for a SKU in a region (mutating)."""
    return (
        f"[demo] Updated forecast for {sku} in {region}: "
        f"projected_units={units} (write applied)"
    )


@mcp.tool
def create_product_entry(sku: str, name: str, category: str, list_price: float) -> str:
    """Add a new product to the catalog (mutating)."""
    return (
        f"[demo] Created product {sku}: name={name}, category={category}, "
        f"list_price={list_price:.2f}"
    )


@mcp.tool
def delete_product_entry(sku: str) -> str:
    """Remove a product from the catalog (mutating)."""
    return f"[demo] Deleted product {sku} from catalog (write applied)"


if __name__ == "__main__":
    mcp.run()
