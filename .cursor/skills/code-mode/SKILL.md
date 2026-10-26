---
name: code-mode
description: >-
  Orchestrates MCP and other tools by writing code that composes calls, filters
  intermediate data, and returns only the final value. Use when a task needs
  several dependent tool calls, a large tool catalog, loops, branching, or
  result shaping; when the user mentions Code Mode, MCP tool use, or Cloudflare
  Agents; and when building an agent that should expose tools as a typed API
  instead of one tool call per turn.
---

# Code Mode

LLMs write code more reliably than they emit tool-call tokens. Tool-call tokens barely appear in training data. TypeScript and JavaScript do. Present tools as a typed API, let the model write one program, and run that program so intermediate results stay out of the model context.

Rationale: https://blog.cloudflare.com/code-mode/
Current API: https://developers.cloudflare.com/agents/tools/codemode/

Code Mode changes how results move. It does not change what the agent is allowed to do.

## Pick the pattern

Use one direct tool call when the task is a single known operation and the result is small.

Use Code Mode when the task needs any of these:

- two or more dependent calls
- a loop, branch, or filter over tool results
- a catalog large enough that loading every schema would dominate the prompt
- joining, aggregating, or shaping results before the answer

A chain of tool calls whose only job is to copy a field from one result into the next call is the failure this skill exists to prevent.

## While using tools in Cursor

The Cursor harness is the supervisor. MCP tools are not importable from a shell script. Apply the pattern inside the agent loop.

1. Discover narrowly. Inspect one namespace or one tool schema before calling it. Do not load every namespace.
2. Send independent calls in one turn. Wait only when a later call needs an earlier result.
3. Keep intermediate payloads out of the next turn. When a result is large, run a short program that extracts the fields the next step needs, and pass only those fields on.
4. Return the value the user asked for. Do not narrate rows the program already discarded.
5. Read credentials from the environment the host already has. Never print a key, token, or connection string, and never write one into code, docs, or a commit.

## Use the connected catalog

Follow `mcp-purpose-routing` for which namespace handles a purpose. That list is the ready set for this Cursor account (2026-10-26). A namespace in `needsAuth` or `error` is not a tool you can call.

1. Discover one namespace before the first call. Do not load the full catalog. It is hundreds of kilobytes.
2. Large ready namespaces stay closed until you need one tool: `plugin-vercel-vercel` (239), `user-higgsfield` (186), `plugin-elevenlabs-elevenlabs` (164), `user-neon` (120), `user-fal` (65). Fetch that tool's schema, then call it.
3. One direct call when the operation is known and the result is small.
4. When several ready calls are independent, send them in one turn. When the next call needs a field from the previous result, keep only that field. Do not paste the raw payload into the next prompt.
5. A page or PDF goes through `user-jina` `read_url` or `extract_pdf`, or the next ready extract peer, then `user-sqz` `compress` if the markdown is still bulky.
6. Before multi-step work, `user-mem0-mcp` `search_memories`. After a decision, `add_memory`. Stored user ids on this account are `vishnu`, `vishnubalachandran`, `vishnubalachandran596`, and `mem0-mcp`.
7. Do not call `mcp_auth` while doing the task.

Do not add a Workers runtime to an existing app just to follow this skill. Use the loop above unless the user is building an agent.

## When you are building the agent

Package: `@cloudflare/codemode`. The September 2025 snippet `import { codemode } from "agents/codemode/ai"` is not the current package.

Code Mode is experimental. Say so when you add it to a production path.

Defaults:

- One code-execution tool. The model writes JavaScript against typed methods.
- `DynamicWorkerExecutor` with a Worker Loader binding named `LOADER`, `nodejs_compat`, and `compatibility_date` set to the day you ship.
- No general `fetch` or `connect` in the sandbox. External access goes through the tool bindings. The host holds the credentials. Generated code never receives them.
- Create one isolate per snippet and discard it. Do not pool or prewarm.
- `createCodeTool()` from `@cloudflare/codemode/ai` for a stateless AI SDK tool set. It drops tools with `needsApproval`. Each sandbox-callable tool needs an `execute` function.
- `toolSetConnector()` plus `createCodemodeRuntime()` when a call must pause for approval, or when the catalog should be discovered with `codemode.search()` and `codemode.describe()`.
- `codeMcpServer()` from `@cloudflare/codemode/mcp` to wrap an existing MCP server as one `code` tool. It returns an SDK v1 server. Serve it with `createLegacyMcpHandler`. It does not pause per tool, so authorize inside each upstream handler. For durable approval on an Agents SDK connection, use `McpConnector` and set `requiresApproval` on side-effecting methods.
- `openApiMcpServer()` when the API is too large to put every operation in the tool description. Search code reads the spec. Execute code calls `codemode.request(...)`. The host adds the credential.

Install: `npm i @cloudflare/codemode agents ai zod`

Worker binding:

```jsonc
{
  "compatibility_date": "2026-10-26",
  "compatibility_flags": ["nodejs_compat"],
  "worker_loaders": [{ "binding": "LOADER" }]
}
```

Stateless AI SDK tool:

```ts
import { DynamicWorkerExecutor } from "@cloudflare/codemode";
import { createCodeTool } from "@cloudflare/codemode/ai";
import { generateText, stepCountIs } from "ai";

const executor = new DynamicWorkerExecutor({ loader: env.LOADER });
const codemode = createCodeTool({ tools: weatherTools, executor });

const response = await generateText({
  model,
  prompt,
  tools: { codemode },
  stopWhen: stepCountIs(5),
});
```

Namespaced providers use unique JavaScript identifiers. A custom `description` may include `{{types}}` where the generated TypeScript should appear.

```ts
createCodeTool({
  tools: [
    { name: "weather", tools: weatherTools },
    { name: "notifications", tools: notificationTools },
  ],
  executor,
});
```

Durable runtime. Drop the manual `CodemodeRuntime` export when the Code Mode Vite plugin adds it. Resume a paused run with `runtime.approve({ executionId })`. A function-valued `needsApproval` always requires approval.

```ts
import {
  createCodemodeRuntime,
  DynamicWorkerExecutor,
} from "@cloudflare/codemode";
import { toolSetConnector } from "@cloudflare/codemode/ai";

export { CodemodeRuntime } from "@cloudflare/codemode";

const operations = toolSetConnector(this.ctx, {
  name: "operations",
  instructions: "Use these tools to manage customer requests.",
  tools: operationTools,
});

const runtime = createCodemodeRuntime({
  ctx: this.ctx,
  executor: new DynamicWorkerExecutor({ loader: this.env.LOADER }),
  connectors: [operations],
});
```

Sandbox discovery:

| Method | Role |
| --- | --- |
| `codemode.search(query)` | Ranked methods and snippets, capped at 50 |
| `codemode.describe(target)` | TypeScript for one connector, method, or snippet |
| `codemode.step(name, fn)` | Run a closure once; replay returns the recorded result |
| `codemode.run(name, input?)` | Run a saved snippet |

Search before calling an unfamiliar method.

Wrap an existing MCP server:

```ts
import { DynamicWorkerExecutor } from "@cloudflare/codemode";
import { codeMcpServer } from "@cloudflare/codemode/mcp";
import { createLegacyMcpHandler } from "agents/mcp";

const executor = new DynamicWorkerExecutor({ loader: env.LOADER });
const server = await codeMcpServer({ server: upstream, executor });
return createLegacyMcpHandler(server, { route: "/mcp" })(request, env, ctx);
```

`structuredContent` is returned directly. Text-only content is joined and parsed as JSON when possible. Upstream MCP errors become exceptions. A final response past about 6,000 estimated tokens is cut and marked `--- TRUNCATED ---`.

## Sandbox code shape

```js
async () => {
  const rows = await codemode.list_orders({ status: "open" });
  return rows
    .filter((row) => row.total > 100)
    .map(({ id, total }) => ({ id, total }));
};
```

Call connectors one after another when an execution can pause for approval. `Promise.all` is only for calls that cannot pause.

Large HTTP APIs use `codemode.request` inside the execute tool. The token stays in the host.

```js
async () => {
  const response = await codemode.request({
    method: "GET",
    path: "/orders",
    query: { status: "processing", limit: 20 },
  });
  return response.items.map(({ id, status }) => ({ id, status }));
};
```

MCP portal clients that want search-and-execute connect with `?codemode=search_and_execute`. Hyphens and dots in tool names become underscores inside the sandbox.

Guides:

- https://developers.cloudflare.com/agents/tools/codemode/ai-sdk/
- https://developers.cloudflare.com/agents/tools/codemode/durable-runtime/
- https://developers.cloudflare.com/agents/tools/codemode/mcp/
- https://developers.cloudflare.com/agents/model-context-protocol/guides/build-codemode-mcp-server/
- https://developers.cloudflare.com/agents/model-context-protocol/guides/build-codemode-openapi-mcp-server/
