# Verdandi

Verdandi is a pure Go local orchestration runtime for turning natural-language
requests into small, inspectable workflows. It provides a CLI plus MCP stdio and
Streamable HTTP transports, so the same local runtime can be used from a
terminal, a local MCP client, or an HTTP-capable MCP gateway.

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

Choose a language:

- [English](README.en.md)
- [한국어](README.ko.md)
