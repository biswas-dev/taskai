import express from "express";
import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StreamableHTTPServerTransport } from "@modelcontextprotocol/sdk/server/streamableHttp.js";
import { z } from "zod";
import { TaskAIClient, Task, Project, SwimLane, Comment, WikiPage, WikiBlock, User } from "./api.js";

const TASKAI_API_URL = process.env.TASKAI_API_URL || "https://taskai.cc";
const PORT = parseInt(process.env.PORT || "3000", 10);

/** Map known MCP client identifiers to friendly display names. */
const AGENT_NAME_MAP: Record<string, string> = {
  "claude-code": "Claude Code",
  "codex-cli": "Codex",
  "gemini-cli": "Gemini",
  "cursor": "Cursor",
  "windsurf": "Windsurf",
};

function normalizeAgentName(raw: string): string {
  const key = raw.trim().toLowerCase();
  return AGENT_NAME_MAP[key] ?? raw.trim().slice(0, 100);
}

// --- API key validation cache (5-minute TTL) ---
interface CacheEntry { user: User; validUntil: number; agentName?: string }
const apiKeyCache = new Map<string, CacheEntry>();
const CACHE_TTL_MS = 5 * 60 * 1000;

// --- Persistent agent name cache (survives container restarts) ---
import { readFileSync, writeFileSync, mkdirSync } from "fs";
const AGENT_CACHE_PATH = "/tmp/taskai-mcp-agents.json";

function loadAgentCache(): Record<string, string> {
  try { return JSON.parse(readFileSync(AGENT_CACHE_PATH, "utf-8")); }
  catch { return {}; }
}

function saveAgentName(apiKeyHash: string, agentName: string): void {
  const cache = loadAgentCache();
  cache[apiKeyHash] = agentName;
  try { writeFileSync(AGENT_CACHE_PATH, JSON.stringify(cache)); } catch { /* best-effort */ }
}

function getPersistedAgentName(apiKeyHash: string): string | undefined {
  return loadAgentCache()[apiKeyHash];
}

/** Simple hash to avoid storing raw API keys on disk */
function hashKey(key: string): string {
  let h = 0;
  for (let i = 0; i < key.length; i++) {
    h = ((h << 5) - h + key.charCodeAt(i)) | 0;
  }
  return h.toString(36);
}

/**
 * Helper to format response with minimal tokens by default.
 * Use verbose=true to get full details with pretty formatting.
 */
function formatResponse(data: unknown, verbose = false): string {
  return verbose ? JSON.stringify(data, null, 2) : JSON.stringify(data);
}

/**
 * Extract minimal fields from a task for list operations.
 */
function minimizeTask(task: Task) {
  return {
    id: task.id,
    task_number: task.task_number,
    title: task.title,
    status: task.status,
    priority: task.priority,
    swim_lane_name: task.swim_lane_name,
    assignee_name: task.assignee_name,
  };
}

/**
 * Extract minimal fields from a project for list operations.
 */
function minimizeProject(project: Project) {
  return {
    id: project.id,
    name: project.name,
  };
}

/**
 * Extract minimal fields from a swim lane.
 */
function minimizeSwimLane(lane: SwimLane) {
  return {
    id: lane.id,
    name: lane.name,
    status_category: lane.status_category,
  };
}

/**
 * Extract minimal fields from a comment.
 */
function minimizeComment(comment: Comment) {
  return {
    id: comment.id,
    content: comment.content,
    author_id: comment.author_id,
    created_at: comment.created_at,
  };
}

/**
 * Extract minimal fields from a wiki page for list operations.
 */
function minimizeWikiPage(page: WikiPage) {
  return { id: page.id, title: page.title, slug: page.slug, updated_at: page.updated_at };
}

/**
 * Extract minimal fields from a wiki search block.
 */
function minimizeWikiBlock(block: WikiBlock) {
  return { page_id: block.page_id, page_title: block.page_title, block_type: block.block_type, snippet: block.snippet };
}

/**
 * Create and configure the MCP server with all TaskAI tools.
 */
function createServer(client: TaskAIClient, cachedUser?: User): McpServer {
  const server = new McpServer({
    name: "taskai",
    version: "1.0.0",
  });

  // --- get_me ---
  server.tool(
    "get_me",
    "Get current authenticated user info",
    { verbose: z.boolean().optional().describe("Return full details (default: false)") },
    async ({ verbose }) => {
      const user = cachedUser ?? await client.getMe();
      return { content: [{ type: "text", text: formatResponse(user, verbose) }] };
    }
  );

  // --- list_projects ---
  server.tool(
    "list_projects",
    "List all projects (minimal fields by default, use verbose=true for full details)",
    {
      page: z.number().optional(),
      limit: z.number().optional(),
      verbose: z.boolean().optional().describe("Return full details (default: false)"),
    },
    async ({ page, limit, verbose }) => {
      const result = await client.listProjects(page, limit);
      const data = verbose
        ? result
        : { projects: result.projects.map(minimizeProject), total: result.total };
      return { content: [{ type: "text", text: formatResponse(data, verbose) }] };
    }
  );

  // --- get_project ---
  server.tool(
    "get_project",
    "Get project details by ID",
    {
      project_id: z.string().describe("Project ID"),
      verbose: z.boolean().optional().describe("Pretty print JSON (default: false)"),
    },
    async ({ project_id, verbose }) => {
      const project = await client.getProject(project_id);
      return { content: [{ type: "text", text: formatResponse(project, verbose) }] };
    }
  );

  // --- list_swim_lanes ---
  server.tool(
    "list_swim_lanes",
    "List swim lanes (columns) for a project. Each lane has a status_category (todo, in_progress, done) that determines task status. Returns minimal fields by default.",
    {
      project_id: z.string().describe("Project ID"),
      verbose: z.boolean().optional().describe("Return full details (default: false)"),
    },
    async ({ project_id, verbose }) => {
      const lanes = await client.listSwimLanes(project_id);
      const data = verbose ? lanes : lanes.map(minimizeSwimLane);
      return { content: [{ type: "text", text: formatResponse(data, verbose) }] };
    }
  );

  // --- list_tasks ---
  server.tool(
    "list_tasks",
    "List tasks in a project (optional status/search filter). Returns minimal fields by default (id, task_number, title, status, priority). Use verbose=true for full task details.",
    {
      project_id: z.string().describe("Project ID"),
      query: z.string().optional().describe("Search query"),
      status: z.string().optional().describe("Filter by status (e.g. todo, in_progress, done)"),
      page: z.number().optional(),
      limit: z.number().optional(),
      verbose: z.boolean().optional().describe("Return full task details (default: false)"),
    },
    async ({ project_id, query, status, page, limit, verbose }) => {
      const result = await client.listTasks(project_id, { query, status, page, limit });
      const data = verbose
        ? result
        : { tasks: result.tasks.map(minimizeTask), total: result.total };
      return { content: [{ type: "text", text: formatResponse(data, verbose) }] };
    }
  );

  // --- get_task ---
  server.tool(
    "get_task",
    "Get a single task by its project-scoped task number",
    {
      project_id: z.string().describe("Project ID"),
      task_number: z.number().describe("Task number within the project (e.g. 1, 2, 3)"),
      verbose: z.boolean().optional().describe("Pretty print JSON (default: false)"),
    },
    async ({ project_id, task_number, verbose }) => {
      const task = await client.getTaskByNumber(project_id, task_number);
      return { content: [{ type: "text", text: formatResponse(task, verbose) }] };
    }
  );

  // --- create_task ---
  server.tool(
    "create_task",
    "Create a new task in a project",
    {
      project_id: z.string().describe("Project ID"),
      title: z.string().describe("Task title"),
      description: z.string().optional().describe("Task description"),
      status: z.string().optional().describe("Task status (default: todo)"),
      priority: z.string().optional().describe("Priority: low, medium, high, critical"),
      assigned_to: z.string().optional().describe("User ID to assign"),
      swim_lane_id: z.number().optional().describe("Swim lane ID (use list_swim_lanes to get valid IDs)"),
      verbose: z.boolean().optional().describe("Pretty print JSON (default: false)"),
    },
    async ({ project_id, title, description, status, priority, assigned_to, swim_lane_id, verbose }) => {
      const task = await client.createTask(project_id, { title, description, status, priority, assigned_to, swim_lane_id });
      const data = verbose ? task : minimizeTask(task);
      return { content: [{ type: "text", text: formatResponse(data, verbose) }] };
    }
  );

  // --- update_task ---
  server.tool(
    "update_task",
    "Update an existing task",
    {
      task_id: z.string().describe("Task ID"),
      title: z.string().optional().describe("New title"),
      description: z.string().optional().describe("New description"),
      status: z.string().optional().describe("New status"),
      priority: z.string().optional().describe("New priority"),
      assigned_to: z.string().optional().describe("New assignee user ID"),
      swim_lane_id: z.number().optional().describe("Swim lane ID (use list_swim_lanes to get valid IDs)"),
      verbose: z.boolean().optional().describe("Pretty print JSON (default: false)"),
    },
    async ({ task_id, title, description, status, priority, assigned_to, swim_lane_id, verbose }) => {
      const task = await client.updateTask(task_id, { title, description, status, priority, assigned_to, swim_lane_id });
      const data = verbose ? task : minimizeTask(task);
      return { content: [{ type: "text", text: formatResponse(data, verbose) }] };
    }
  );

  // --- list_comments ---
  server.tool(
    "list_comments",
    "List comments on a task. Returns minimal fields by default.",
    {
      task_id: z.string().describe("Task ID"),
      verbose: z.boolean().optional().describe("Return full details (default: false)"),
    },
    async ({ task_id, verbose }) => {
      const result = await client.listComments(task_id);
      const data = verbose
        ? result
        : { comments: result.comments.map(minimizeComment) };
      return { content: [{ type: "text", text: formatResponse(data, verbose) }] };
    }
  );

  // --- add_comment ---
  server.tool(
    "add_comment",
    "Add a comment to a task",
    {
      task_id: z.string().describe("Task ID"),
      content: z.string().describe("Comment text"),
      verbose: z.boolean().optional().describe("Pretty print JSON (default: false)"),
    },
    async ({ task_id, content, verbose }) => {
      const comment = await client.addComment(task_id, content);
      return { content: [{ type: "text", text: formatResponse(comment, verbose) }] };
    }
  );

  // --- update_comment ---
  server.tool(
    "update_comment",
    "Update a comment. Only the comment owner, project owner, or super admin can update.",
    {
      comment_id: z.string().describe("Comment ID"),
      content: z.string().describe("New comment text"),
      verbose: z.boolean().optional().describe("Pretty print JSON (default: false)"),
    },
    async ({ comment_id, content, verbose }) => {
      const comment = await client.updateComment(comment_id, content);
      return { content: [{ type: "text", text: formatResponse(comment, verbose) }] };
    }
  );

  // --- delete_comment ---
  server.tool(
    "delete_comment",
    "Delete a comment. Only the comment owner, project owner, or super admin can delete.",
    {
      comment_id: z.string().describe("Comment ID"),
    },
    async ({ comment_id }) => {
      const result = await client.deleteComment(comment_id);
      return { content: [{ type: "text", text: formatResponse(result, false) }] };
    }
  );

  // --- list_project_drawings ---
  server.tool(
    "list_project_drawings",
    "List all drawings registered to a project",
    {
      project_id: z.string().describe("Project ID"),
      verbose: z.boolean().optional().describe("Pretty print JSON (default: false)"),
    },
    async ({ project_id, verbose }) => {
      const drawings = await client.listProjectDrawings(project_id);
      return { content: [{ type: "text", text: formatResponse(drawings, verbose) }] };
    }
  );

  // --- create_drawing ---
  server.tool(
    "create_drawing",
    "Create a new drawing canvas and register it with a project. Optionally pre-populate with a scene. Returns the draw_id and the shortcode to embed it in wiki pages: [draw:ID:edit:m]",
    {
      project_id: z.string().describe("Project ID to register the drawing with"),
      title: z.string().optional().describe("Drawing title (default: 'Untitled')"),
      scene: z.record(z.unknown()).optional().describe("Scene JSON: {version:1, elements:[...]}. Elements: rect/ellipse (x,y,w,h,text), arrow/line (x,y,x2,y2), text (x,y,w,h,text,fontSize), pencil (pts:[{x,y}]). Fields: strokeColor, fillColor, opacity(0-100), strokeWidth(1-4), angle(radians)."),
      verbose: z.boolean().optional().describe("Pretty print JSON (default: false)"),
    },
    async ({ project_id, title, scene, verbose }) => {
      const result = await client.createDrawing(project_id, (title || scene) ? { title, scene } : undefined);
      return { content: [{ type: "text", text: formatResponse(result, verbose) }] };
    }
  );

  // --- save_drawing ---
  server.tool(
    "save_drawing",
    "Save/update an existing drawing's scene data. Use this to programmatically set diagram content.",
    {
      draw_id: z.string().describe("Drawing ID (from create_drawing)"),
      title: z.string().describe("Drawing title"),
      scene: z.record(z.unknown()).describe("Scene JSON: {version:1, elements:[...]}. Elements: rect/ellipse (x,y,w,h,text), arrow/line (x,y,x2,y2), text (x,y,w,h,text,fontSize). Fields: strokeColor, fillColor, opacity(0-100), strokeWidth(1-4)."),
      verbose: z.boolean().optional().describe("Pretty print JSON (default: false)"),
    },
    async ({ draw_id, title, scene, verbose }) => {
      const result = await client.saveDrawing(draw_id, title, scene);
      return { content: [{ type: "text", text: formatResponse(result, verbose) }] };
    }
  );

  // --- get_drawing ---
  server.tool(
    "get_drawing",
    "Get a drawing's current scene data (id, title, scene JSON)",
    {
      draw_id: z.string().describe("Drawing ID"),
      verbose: z.boolean().optional().describe("Pretty print JSON (default: false)"),
    },
    async ({ draw_id, verbose }) => {
      const result = await client.getDrawing(draw_id);
      return { content: [{ type: "text", text: formatResponse(result, verbose) }] };
    }
  );

  // --- search_wiki ---
  server.tool(
    "search_wiki",
    "Search wiki content across pages (full-text search)",
    {
      query: z.string().describe("Search query"),
      project_id: z.string().optional().describe("Filter by project ID"),
      limit: z.number().optional().describe("Max results (default: 20, max: 100)"),
      recency_days: z.number().optional().describe("Only return pages updated in last N days"),
      verbose: z.boolean().optional().describe("Pretty print JSON (default: false)"),
    },
    async ({ query, project_id, limit, recency_days, verbose }) => {
      const result = await client.searchWiki({ query, project_id, limit, recency_days });
      const data = verbose ? result : { results: result.results.map(minimizeWikiBlock), total: result.total };
      return { content: [{ type: "text", text: formatResponse(data, verbose) }] };
    }
  );

  // --- list_wiki_pages ---
  server.tool(
    "list_wiki_pages",
    "List all wiki pages in a project",
    {
      project_id: z.string().describe("Project ID"),
      verbose: z.boolean().optional().describe("Pretty print JSON (default: false)"),
    },
    async ({ project_id, verbose }) => {
      const pages = await client.listWikiPages(project_id);
      const data = verbose ? pages : pages.map(minimizeWikiPage);
      return { content: [{ type: "text", text: formatResponse(data, verbose) }] };
    }
  );

  // --- get_wiki_page ---
  server.tool(
    "get_wiki_page",
    "Get a specific wiki page by ID including its full markdown content",
    {
      page_id: z.string().describe("Wiki page ID"),
      verbose: z.boolean().optional().describe("Pretty print JSON (default: false)"),
    },
    async ({ page_id, verbose }) => {
      const [page, pageContent] = await Promise.all([
        client.getWikiPage(page_id),
        client.getWikiPageContent(page_id),
      ]);
      const result = { ...page, content: pageContent.content };
      return { content: [{ type: "text", text: formatResponse(result, verbose) }] };
    }
  );

  // --- get_wiki_page_content ---
  server.tool(
    "get_wiki_page_content",
    "Get the markdown content of a wiki page",
    {
      page_id: z.string().describe("Wiki page ID"),
      verbose: z.boolean().optional().describe("Pretty print JSON (default: false)"),
    },
    async ({ page_id, verbose }) => {
      const result = await client.getWikiPageContent(page_id);
      return { content: [{ type: "text", text: formatResponse(result, verbose) }] };
    }
  );

  // --- create_wiki_page ---
  server.tool(
    "create_wiki_page",
    "Create a new wiki page in a project. Content supports markdown with extensions: references ([^1] inline → superscript citation, [^1]: text → reference list), graph links ([[wiki:ID|Label]], [[task:ID|Label]]), drawings ([draw:id]), and Figma embeds ([figma:url]).",
    {
      project_id: z.string().describe("Project ID"),
      title: z.string().describe("Page title"),
      content: z.string().optional().describe("Initial page content (markdown). Supports references: use [^N] for inline citations and [^N]: text for definitions"),
      verbose: z.boolean().optional().describe("Pretty print JSON (default: false)"),
    },
    async ({ project_id, title, content, verbose }) => {
      const page = await client.createWikiPage(project_id, title);
      if (content) {
        await client.updateWikiPageContent(String(page.id), content);
      }
      return { content: [{ type: "text", text: formatResponse(verbose ? page : minimizeWikiPage(page), verbose) }] };
    }
  );

  // --- update_wiki_page_content ---
  server.tool(
    "update_wiki_page_content",
    "Update the content of an existing wiki page. Content supports markdown with extensions: references ([^1] inline → superscript citation, [^1]: text → reference list), graph links ([[wiki:ID|Label]], [[task:ID|Label]]), drawings ([draw:id]), and Figma embeds ([figma:url]).",
    {
      page_id: z.string().describe("Wiki page ID"),
      content: z.string().describe("New page content (markdown). Supports references: use [^N] for inline citations and [^N]: text for definitions"),
      verbose: z.boolean().optional().describe("Pretty print JSON (default: false)"),
    },
    async ({ page_id, content, verbose }) => {
      const page = await client.updateWikiPageContent(page_id, content);
      return { content: [{ type: "text", text: formatResponse(verbose ? page : minimizeWikiPage(page), verbose) }] };
    }
  );

  // --- autocomplete_wiki_pages ---
  server.tool(
    "autocomplete_wiki_pages",
    "Autocomplete wiki page titles (fuzzy search)",
    {
      query: z.string().describe("Search query for page title"),
      project_id: z.string().optional().describe("Filter by project ID"),
      limit: z.number().optional().describe("Max results (default: 10, max: 50)"),
      verbose: z.boolean().optional().describe("Pretty print JSON (default: false)"),
    },
    async ({ query, project_id, limit, verbose }) => {
      const results = await client.autocompletePages(query, project_id, limit);
      return { content: [{ type: "text", text: formatResponse(results, verbose) }] };
    }
  );

  // --- get_version ---
  server.tool(
    "get_version",
    "Get system version information (backend version, DB migration version, build info)",
    {
      verbose: z.boolean().optional().describe("Pretty print JSON (default: false)"),
    },
    async ({ verbose }) => {
      const version = await client.getVersion();
      return { content: [{ type: "text", text: formatResponse(version, verbose) }] };
    }
  );

  // --- health_check ---
  server.tool(
    "health_check",
    "Check system health status (database connectivity)",
    {
      verbose: z.boolean().optional().describe("Pretty print JSON (default: false)"),
    },
    async ({ verbose }) => {
      const health = await client.healthCheck();
      return { content: [{ type: "text", text: formatResponse(health, verbose) }] };
    }
  );

  return server;
}

// --- Express app ---
const app = express();
app.use(express.json());

// Health endpoint
app.get("/health", (_req, res) => {
  res.json({ status: "ok", service: "taskai-mcp" });
});

// MCP endpoint — stateless: one transport per request
app.post("/mcp", async (req, res) => {
  // Extract API key from X-API-Key header
  const apiKey = req.headers["x-api-key"] as string | undefined;
  if (!apiKey) {
    res.status(401).json({ error: "Missing X-API-Key header" });
    return;
  }

  // Validate API key — use cache to avoid round-trip on every request
  const client = new TaskAIClient(TASKAI_API_URL, apiKey);
  let cachedUser: User | undefined;
  const now = Date.now();
  const cached = apiKeyCache.get(apiKey);
  if (cached && cached.validUntil > now) {
    cachedUser = cached.user;
  } else {
    try {
      cachedUser = await client.getMe();
      apiKeyCache.set(apiKey, { user: cachedUser, validUntil: now + CACHE_TTL_MS });
    } catch {
      res.status(403).json({ error: "Invalid API key" });
      return;
    }
  }

  // Detect agent name from multiple sources (in priority order):
  // 1. X-Agent-Name HTTP header (explicit, most reliable when sent)
  // 2. MCP initialize message clientInfo.name
  // 3. User-Agent header (fallback: detect known MCP clients)
  // 4. Cached from a previous request with the same API key
  const headerAgent = req.headers["x-agent-name"] as string | undefined;
  if (headerAgent) {
    client.agentName = normalizeAgentName(headerAgent);
  }

  if (!client.agentName) {
    const messages = Array.isArray(req.body) ? req.body : [req.body];
    for (const msg of messages) {
      if (msg?.method === "initialize" && msg?.params?.clientInfo?.name) {
        client.agentName = normalizeAgentName(msg.params.clientInfo.name);
        break;
      }
    }
  }

  // Detect agent from User-Agent header (MCP clients often identify themselves)
  if (!client.agentName) {
    const ua = (req.headers["user-agent"] || "").toLowerCase();
    for (const [key, name] of Object.entries(AGENT_NAME_MAP)) {
      if (ua.includes(key)) {
        client.agentName = name;
        break;
      }
    }
  }

  if (!client.agentName) {
    const entry = apiKeyCache.get(apiKey);
    if (entry?.agentName) client.agentName = entry.agentName;
  }

  // Final fallback: persistent file cache (survives container restarts)
  const keyHash = hashKey(apiKey);
  if (!client.agentName) {
    const persisted = getPersistedAgentName(keyHash);
    if (persisted) client.agentName = persisted;
  }

  // Persist agent name in both memory and file cache
  if (client.agentName) {
    const entry = apiKeyCache.get(apiKey);
    if (entry) entry.agentName = client.agentName;
    saveAgentName(keyHash, client.agentName);
  }

  // Create MCP server with authenticated client and cached user
  const server = createServer(client, cachedUser);

  // Stateless transport — no session persistence
  const transport = new StreamableHTTPServerTransport({
    sessionIdGenerator: undefined,
  });
  await server.connect(transport);
  await transport.handleRequest(req, res, req.body);
});

// Handle GET and DELETE on /mcp for protocol compliance (stateless = 405)
app.get("/mcp", (_req, res) => {
  res.status(405).json({ error: "Method not allowed — stateless server, use POST" });
});

app.delete("/mcp", (_req, res) => {
  res.status(405).json({ error: "Method not allowed — stateless server, use POST" });
});

app.listen(PORT, () => {
  console.log(`TaskAI MCP server listening on port ${PORT}`);
  console.log(`API backend: ${TASKAI_API_URL}`);
});
