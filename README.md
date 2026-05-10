# Verdandi

Verdandi is a pure Go MCP runtime for turning natural-language requests into
workflow packages and persistent agent/skill assets for external LLM coding
agents. It provides a CLI plus MCP stdio and Streamable HTTP transports, so the
same local asset registry can be used from a terminal, a local MCP client, or an
HTTP-capable MCP gateway.

## MCP Server

Verdandi's MCP server targets protocol version `2025-11-25` and exposes tools,
resources, resource templates, and prompts without client-specific behavior over
stdio or `POST /mcp` Streamable HTTP. See
[docs/mcp-standard-compatibility.md](docs/mcp-standard-compatibility.md) for the
standard compatibility surface and
[docs/mcp-inspector-fixtures.jsonl](docs/mcp-inspector-fixtures.jsonl) for
JSON-RPC fixture requests.

## Spinning Wheel

Spinning Wheel is an optional visualizer for Verdandi runs. It shows agent creation, movement, decisions, speech bubbles, and live event streaming. See [docs/spinning-wheel.md](docs/spinning-wheel.md) for setup and operation.

## Upgrade

After installing a release, upgrade from GitHub Releases with:

```bash
verdandi upgrade
```

Use `verdandi upgrade --dry-run` to preview the selected archive, or
`verdandi upgrade --version 0.0.2 --install-dir "$HOME/.local/bin"` to install a
specific version and location.

Choose a language:

- [English](README.en.md)
- [한국어](README.ko.md)
