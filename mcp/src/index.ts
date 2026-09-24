import express from "express";
import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StreamableHTTPServerTransport } from "@modelcontextprotocol/sdk/server/streamableHttp.js";
import { z } from "zod";
import { TaskAIClient, Task, Project, SwimLane, Comment, WikiPage, WikiBlock, User, Milestone, WikiAnnotation, ProjectMember, WikiSharing } from "./api.js";
import { ToolInputError, addProjectMember, updateWikiSharing } from "./membership.js";

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
  return { id: page.id, title: page.title, slug: page.slug, parent_id: page.parent_id ?? null, updated_at: page.updated_at };
}

/**
 * Extract minimal fields from a wiki search block.
 */
function minimizeWikiBlock(block: WikiBlock) {
  return { page_id: block.page_id, page_title: block.page_title, headings_path: block.headings_path, snippet: block.snippet };
}

/**
 * Extract minimal fields from a wiki annotation.
 */
function minimizeWikiAnnotation(annotation: WikiAnnotation) {
  return {
    id: annotation.id,
    page_id: annotation.page_id,
    selected_text: annotation.selected_text,
    color: annotation.color,
    resolved: annotation.resolved,
    comments_count: annotation.comments?.length ?? 0,
  };
}

/**
 * Extract minimal fields from a project member. user_id (not the membership id)
 * is what the sharing tools take.
 */
function minimizeProjectMember(member: ProjectMember) {
  return { user_id: member.user_id, email: member.email, name: member.name ?? null, role: member.role };
}

/**
 * Minimal view of a page's sharing: the public link token is reduced to a flag.
 */
function minimizeWikiSharing(sharing: WikiSharing) {
  return {
    visibility: sharing.visibility,
    can_manage: sharing.can_manage,
    created_by: sharing.created_by,
    shared_with: sharing.shared_with.map((u) => ({ user_id: u.user_id, email: u.email, name: u.user_name ?? null })),
    public_link: !!sharing.public_token,
  };
}

/**
 * Report an actionable input problem as a tool error; rethrow anything else so
 * API failures surface the way they do for every other tool.
 */
function toolError(err: unknown) {
  if (err instanceof ToolInputError) {
    return { content: [{ type: "text" as const, text: err.message }], isError: true };
  }
  throw err;
}

/**
 * Create and configure the MCP server with all TaskAI tools.
 */
function createServer(client: TaskAIClient, cachedUser?: User, defaultProjectIds?: string[]): McpServer {
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

  // --- list_project_members ---
  server.tool(
    "list_project_members",
    "List the members of a project: user_id, email, name and role. Use it to find someone's user_id or exact email before sharing a wiki page with them (pages can only be shared with project members) or to check who can be added. Pass user_id, not the membership id, to other tools.",
    {
      project_id: z.string().describe("Project ID"),
      verbose: z.boolean().optional().describe("Return full details including membership id and granted_at (default: false)"),
    },
    async ({ project_id, verbose }) => {
      const members = await client.listProjectMembers(project_id);
      const data = verbose ? members : members.map(minimizeProjectMember);
      return { content: [{ type: "text", text: formatResponse(data, verbose) }] };
    }
  );

  // --- add_project_member ---
  server.tool(
    "add_project_member",
    "Add a person to a project so they can see its tasks and wiki, and so restricted wiki pages can be shared with them. They are added immediately (no invitation to accept). Only the project owner or a project admin can do this. Every project belongs to one team and only that team's members can join it: if the person is not in the team, the call fails unless add_to_team=true, which adds them to the team first. Someone with no TaskAI account can only be invited to the team by email; they must sign up before they can be added. Existing members are left unchanged.",
    {
      project_id: z.string().describe("Project ID"),
      email: z.string().email().optional().describe("Email of the person to add (give this or user_id)"),
      user_id: z.number().int().positive().optional().describe("User ID of the person to add (give this or email)"),
      role: z.enum(["viewer", "member", "editor", "owner"]).optional().describe("Project role (default: member)"),
      add_to_team: z.boolean().optional().describe("If they are not in the project's team, add them to it first (default: false). With an email that has no TaskAI account, this sends them a signup invitation instead."),
    },
    async (args) => {
      try {
        const result = await addProjectMember(client, args);
        return { content: [{ type: "text", text: formatResponse(result) }] };
      } catch (err) {
        return toolError(err);
      }
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

  // --- create_swim_lane ---
  server.tool(
    "create_swim_lane",
    "Create a new swim lane (column) in a project. Max 6 lanes per project.",
    {
      project_id: z.string().describe("Project ID"),
      name: z.string().describe("Lane name (max 50 chars)"),
      status_category: z.enum(["todo", "in_progress", "done"]).describe("Status category — controls task status when moved into this lane"),
      color: z.string().optional().describe("Hex color, e.g. #5e6ad2 (default: #6B7280 gray)"),
      position: z.number().optional().describe("Position (column order). Defaults to 0."),
      verbose: z.boolean().optional().describe("Return full details (default: false)"),
    },
    async ({ project_id, name, status_category, color, position, verbose }) => {
      const lane = await client.createSwimLane(project_id, { name, status_category, color, position });
      const data = verbose ? lane : minimizeSwimLane(lane);
      return { content: [{ type: "text", text: formatResponse(data, verbose) }] };
    }
  );

  // --- update_swim_lane ---
  server.tool(
    "update_swim_lane",
    "Update an existing swim lane (rename, recolor, reorder, or change status category). Note: (project_id, position) is unique — to swap two lanes, first move one to an unused position.",
    {
      swim_lane_id: z.number().describe("Swim lane ID"),
      name: z.string().optional().describe("New name (max 50 chars)"),
      color: z.string().optional().describe("New hex color, e.g. #f59e0b"),
      position: z.number().optional().describe("New position (must be unique within the project)"),
      status_category: z.enum(["todo", "in_progress", "done"]).optional().describe("New status category"),
      verbose: z.boolean().optional().describe("Return full details (default: false)"),
    },
    async ({ swim_lane_id, name, color, position, status_category, verbose }) => {
      const lane = await client.updateSwimLane(swim_lane_id, { name, color, position, status_category });
      const data = verbose ? lane : minimizeSwimLane(lane);
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
  const projectScope = defaultProjectIds?.length ? ` Scoped to project${defaultProjectIds.length > 1 ? "s" : ""} ${defaultProjectIds.join(", ")} by default.` : "";
  server.tool(
    "search_wiki",
    `Search project wiki for architecture docs, decisions, and implementation details. Modes: 'hybrid' (default, best accuracy — combines full-text + vector similarity via reciprocal rank fusion), 'fts' (full-text ranked), 'semantic' (vector similarity — finds conceptually related content even without keyword overlap), 'keyword' (simple substring match).${projectScope}`,
    {
      query: z.string().describe("Search query"),
      project_id: z.string().optional().describe("Search a single project by ID"),
      project_ids: z.array(z.string()).optional().describe(`Search multiple projects by ID${defaultProjectIds?.length ? ` (default: [${defaultProjectIds.join(", ")}])` : ""}`),
      limit: z.number().optional().describe("Max results (default: 10, max: 100)"),
      recency_days: z.number().optional().describe("Only return pages updated in last N days"),
      mode: z.enum(["fts", "semantic", "hybrid", "keyword"]).optional().describe("Search mode (default: hybrid)"),
      verbose: z.boolean().optional().describe("Pretty print JSON (default: false)"),
    },
    async ({ query, project_id, project_ids, limit, recency_days, mode, verbose }) => {
      const effectiveMode = mode ?? "hybrid";
      const effectiveLimit = limit ?? 10;
      // Priority: explicit project_id > explicit project_ids > default from X-Project-ID header
      const effectiveProjectIds = project_id ? [project_id] : (project_ids ?? defaultProjectIds);
      const result = await client.searchWiki({
        query,
        project_id: effectiveProjectIds?.length === 1 ? effectiveProjectIds[0] : undefined,
        project_ids: effectiveProjectIds?.length !== 1 ? effectiveProjectIds : undefined,
        limit: effectiveLimit,
        recency_days,
        mode: effectiveMode,
      });
      const data = verbose ? result : { results: result.results.map(minimizeWikiBlock), total: result.total };
      return { content: [{ type: "text", text: formatResponse(data, verbose) }] };
    }
  );

  // --- reindex_wiki ---
  server.tool(
    "reindex_wiki",
    "Trigger a full re-index of all wiki pages, generating vector embeddings for semantic search. Returns immediately — re-indexing runs in background. Use after first deployment of embeddings or after a model upgrade.",
    {
      verbose: z.boolean().optional().describe("Pretty print JSON (default: false)"),
    },
    async ({ verbose }) => {
      const result = await client.reindexWiki();
      return { content: [{ type: "text", text: formatResponse(result, verbose) }] };
    }
  );

  // --- list_wiki_pages ---
  const defaultPid = defaultProjectIds?.[0];
  server.tool(
    "list_wiki_pages",
    `List all wiki pages in a project${defaultPid ? ` (default: project ${defaultPid})` : ""}. Pages are hierarchical: parent_id is null for top-level pages, otherwise the ID of the parent page (max 6 levels deep).`,
    {
      project_id: z.string().optional().describe(`Project ID${defaultPid ? ` (default: ${defaultPid})` : ""}`),
      verbose: z.boolean().optional().describe("Pretty print JSON (default: false)"),
    },
    async ({ project_id, verbose }) => {
      const effectiveProjectId = project_id ?? defaultPid;
      if (!effectiveProjectId) {
        return { content: [{ type: "text", text: "project_id is required" }], isError: true };
      }
      const pages = await client.listWikiPages(effectiveProjectId);
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

  // --- list_wiki_annotations ---
  server.tool(
    "list_wiki_annotations",
    "List inline highlights and comments for a wiki page. Returns minimal fields by default; use verbose=true for offsets and full comment threads.",
    {
      page_id: z.string().describe("Wiki page ID"),
      include_resolved: z.boolean().optional().describe("Include resolved annotations (default: true)"),
      verbose: z.boolean().optional().describe("Pretty print JSON (default: false)"),
    },
    async ({ page_id, include_resolved, verbose }) => {
      const annotations = await client.listWikiAnnotations(page_id);
      const filtered = include_resolved === false
        ? annotations.filter((annotation) => !annotation.resolved)
        : annotations;
      const data = verbose ? filtered : filtered.map(minimizeWikiAnnotation);
      return { content: [{ type: "text", text: formatResponse(data, verbose) }] };
    }
  );

  // --- create_wiki_annotation ---
  server.tool(
    "create_wiki_annotation",
    "Create an inline wiki highlight, optionally with an initial comment. Offsets are zero-based character offsets in the rendered wiki text.",
    {
      page_id: z.string().describe("Wiki page ID"),
      start_offset: z.number().int().min(0).describe("Start character offset in rendered wiki text"),
      end_offset: z.number().int().min(1).describe("End character offset in rendered wiki text; must be greater than start_offset"),
      selected_text: z.string().min(1).describe("Exact highlighted text"),
      color: z.enum(["yellow", "blue", "green", "red"]).optional().describe("Highlight color (default: yellow)"),
      comment: z.string().max(5000).optional().describe("Optional initial comment"),
      verbose: z.boolean().optional().describe("Pretty print JSON (default: false)"),
    },
    async ({ page_id, start_offset, end_offset, selected_text, color, comment, verbose }) => {
      const annotation = await client.createWikiAnnotation(page_id, {
        start_offset,
        end_offset,
        selected_text,
        color: color ?? "yellow",
        comment,
      });
      const data = verbose ? annotation : minimizeWikiAnnotation(annotation);
      return { content: [{ type: "text", text: formatResponse(data, verbose) }] };
    }
  );

  // --- update_wiki_annotation ---
  server.tool(
    "update_wiki_annotation",
    "Update a wiki annotation's color or resolved state.",
    {
      annotation_id: z.string().describe("Wiki annotation ID"),
      color: z.enum(["yellow", "blue", "green", "red"]).optional().describe("New highlight color"),
      resolved: z.boolean().optional().describe("Set resolved/unresolved state"),
      verbose: z.boolean().optional().describe("Pretty print JSON (default: false)"),
    },
    async ({ annotation_id, color, resolved, verbose }) => {
      const annotation = await client.updateWikiAnnotation(annotation_id, { color, resolved });
      const data = verbose ? annotation : minimizeWikiAnnotation(annotation);
      return { content: [{ type: "text", text: formatResponse(data, verbose) }] };
    }
  );

  // --- delete_wiki_annotation ---
  server.tool(
    "delete_wiki_annotation",
    "Delete a wiki annotation and its comment thread.",
    {
      annotation_id: z.string().describe("Wiki annotation ID"),
    },
    async ({ annotation_id }) => {
      await client.deleteWikiAnnotation(annotation_id);
      return { content: [{ type: "text", text: "Wiki annotation deleted successfully" }] };
    }
  );

  // --- create_wiki_annotation_comment ---
  server.tool(
    "create_wiki_annotation_comment",
    "Add a comment to an existing wiki annotation.",
    {
      annotation_id: z.string().describe("Wiki annotation ID"),
      content: z.string().min(1).max(5000).describe("Comment body"),
      parent_comment_id: z.number().int().optional().describe("Parent comment ID for threaded replies"),
      verbose: z.boolean().optional().describe("Pretty print JSON (default: false)"),
    },
    async ({ annotation_id, content, parent_comment_id, verbose }) => {
      const comment = await client.createWikiAnnotationComment(annotation_id, { content, parent_comment_id });
      return { content: [{ type: "text", text: formatResponse(comment, verbose) }] };
    }
  );

  // --- update_wiki_annotation_comment ---
  server.tool(
    "update_wiki_annotation_comment",
    "Update the body of a wiki annotation comment.",
    {
      comment_id: z.string().describe("Wiki annotation comment ID"),
      content: z.string().min(1).max(5000).describe("New comment body"),
      verbose: z.boolean().optional().describe("Pretty print JSON (default: false)"),
    },
    async ({ comment_id, content, verbose }) => {
      const comment = await client.updateWikiAnnotationComment(comment_id, content);
      return { content: [{ type: "text", text: formatResponse(comment, verbose) }] };
    }
  );

  // --- delete_wiki_annotation_comment ---
  server.tool(
    "delete_wiki_annotation_comment",
    "Delete a wiki annotation comment.",
    {
      comment_id: z.string().describe("Wiki annotation comment ID"),
    },
    async ({ comment_id }) => {
      await client.deleteWikiAnnotationComment(comment_id);
      return { content: [{ type: "text", text: "Wiki annotation comment deleted successfully" }] };
    }
  );

  // --- create_wiki_page ---
  server.tool(
    "create_wiki_page",
    `Create a new wiki page in a project. Content supports markdown with extensions: references ([^1] inline → superscript citation, [^1]: text → reference list), graph links ([[wiki:ID|Label]], [[task:ID|Label]]), drawings ([draw:id]), and Figma embeds ([figma:url]).${defaultPid ? ` Defaults to project ${defaultPid}.` : ""}`,
    {
      project_id: z.string().optional().describe(`Project ID${defaultPid ? ` (default: ${defaultPid})` : ""}`),
      title: z.string().describe("Page title"),
      parent_id: z.string().optional().describe("Parent wiki page ID to nest this page under (omit for a top-level page; max 6 levels deep)"),
      content: z.string().optional().describe("Initial page content (markdown). Supports references: use [^N] for inline citations and [^N]: text for definitions"),
      verbose: z.boolean().optional().describe("Pretty print JSON (default: false)"),
    },
    async ({ project_id, title, parent_id, content, verbose }) => {
      const effectiveProjectId = project_id ?? defaultPid;
      if (!effectiveProjectId) {
        return { content: [{ type: "text", text: "project_id is required" }], isError: true };
      }
      const page = await client.createWikiPage(effectiveProjectId, title, parent_id);
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

  // --- update_wiki_page_title ---
  server.tool(
    "update_wiki_page_title",
    "Rename a wiki page. Updates the title and regenerates the URL slug.",
    {
      page_id: z.string().describe("Wiki page ID"),
      title: z.string().min(1).max(500).describe("New page title (1–500 chars)"),
      verbose: z.boolean().optional().describe("Pretty print JSON (default: false)"),
    },
    async ({ page_id, title, verbose }) => {
      const page = await client.updateWikiPage(page_id, { title });
      return { content: [{ type: "text", text: formatResponse(verbose ? page : minimizeWikiPage(page), verbose) }] };
    }
  );

  // --- move_wiki_page ---
  server.tool(
    "move_wiki_page",
    "Move a wiki page in the hierarchy: nest it under another page or make it top-level. A page cannot be moved under itself or its descendants, and the tree is limited to 6 levels.",
    {
      page_id: z.string().describe("Wiki page ID to move"),
      parent_id: z.string().nullable().describe("New parent page ID, or null to move to the top level"),
      position: z.number().int().min(0).optional().describe("Optional sort position among siblings (0-based). Defaults to last."),
      verbose: z.boolean().optional().describe("Pretty print JSON (default: false)"),
    },
    async ({ page_id, parent_id, position, verbose }) => {
      const data: { parent_id: number | null; position?: number } = {
        parent_id: parent_id === null || parent_id === "" ? null : Number(parent_id),
      };
      if (position !== undefined) data.position = position;
      const page = await client.updateWikiPage(page_id, data);
      return { content: [{ type: "text", text: formatResponse(verbose ? page : minimizeWikiPage(page), verbose) }] };
    }
  );

  // --- get_wiki_sharing ---
  server.tool(
    "get_wiki_sharing",
    "Get who can see a wiki page. visibility 'project' means every project member; 'restricted' means only the page creator, the project owner and the people in shared_with (the creator and owner always have access and are not listed). can_manage says whether you may change it. public_link is true when a read-only public link is on.",
    {
      page_id: z.string().describe("Wiki page ID"),
      verbose: z.boolean().optional().describe("Return the raw API response, including the public link token if you can manage the page (default: false)"),
    },
    async ({ page_id, verbose }) => {
      const sharing = await client.getWikiSharing(page_id);
      return { content: [{ type: "text", text: formatResponse(verbose ? sharing : minimizeWikiSharing(sharing), verbose) }] };
    }
  );

  // --- update_wiki_sharing ---
  server.tool(
    "update_wiki_sharing",
    "Make a wiki page private or project-wide, and choose who it is shared with. visibility 'restricted' limits the page to its creator, the project owner and the people you share it with; 'project' opens it to every project member. Name people by email or user_id; they must be members of the page's project (see list_project_members, add_project_member), otherwise nothing is changed. mode 'add' (default) keeps everyone already shared and adds these people; 'replace' makes these people the whole list (replace with no one: only the creator and project owner keep access). Omitting user_ids and emails keeps the current list. Only the page creator or the project owner can change sharing. Returns the resulting sharing.",
    {
      page_id: z.string().describe("Wiki page ID"),
      visibility: z.enum(["project", "restricted"]).describe("'restricted' = private to the people it is shared with; 'project' = every project member"),
      emails: z.array(z.string().email()).max(200).optional().describe("Emails of project members to share with"),
      user_ids: z.array(z.number().int().positive()).max(200).optional().describe("User IDs of project members to share with"),
      mode: z.enum(["add", "replace"]).optional().describe("'add' (default) adds to the current share list; 'replace' sets the whole list"),
      verbose: z.boolean().optional().describe("Return the raw API response (default: false)"),
    },
    async ({ verbose, ...args }) => {
      try {
        const sharing = await updateWikiSharing(client, args);
        return { content: [{ type: "text", text: formatResponse(verbose ? sharing : minimizeWikiSharing(sharing), verbose) }] };
      } catch (err) {
        return toolError(err);
      }
    }
  );

  // --- download_wiki_pdf ---
  server.tool(
    "download_wiki_pdf",
    "Generate and download a wiki page as PDF (rendered like the web view). Returns base64-encoded PDF data.",
    {
      page_id: z.string().describe("Wiki page ID"),
    },
    async ({ page_id }) => {
      const result = await client.getWikiPagePdf(page_id);
      return {
        content: [
          { type: "text", text: `PDF generated: ${result.filename} (${Math.round(result.data.length * 3 / 4 / 1024)} KB)` },
          { type: "resource", resource: { uri: `data:application/pdf;base64,${result.data}`, mimeType: "application/pdf", text: result.data } },
        ],
      };
    }
  );

  // --- download_wiki_markdown ---
  server.tool(
    "download_wiki_markdown",
    "Download a wiki page as raw Markdown file",
    {
      page_id: z.string().describe("Wiki page ID"),
    },
    async ({ page_id }) => {
      const result = await client.getWikiPageMarkdown(page_id);
      return {
        content: [
          { type: "text", text: `Markdown file: ${result.filename}\n\n${result.content}` },
        ],
      };
    }
  );

  // --- autocomplete_wiki_pages ---
  server.tool(
    "autocomplete_wiki_pages",
    `Autocomplete wiki page titles (fuzzy search)${defaultPid ? ` — scoped to project ${defaultPid} by default` : ""}`,
    {
      query: z.string().describe("Search query for page title"),
      project_id: z.string().optional().describe(`Filter by project ID${defaultPid ? ` (default: ${defaultPid})` : ""}`),
      limit: z.number().optional().describe("Max results (default: 10, max: 50)"),
      verbose: z.boolean().optional().describe("Pretty print JSON (default: false)"),
    },
    async ({ query, project_id, limit, verbose }) => {
      const results = await client.autocompletePages(query, project_id ?? defaultPid, limit);
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

  // --- list_milestones ---
  server.tool(
    "list_milestones",
    "List milestones for a project with task counts. Milestones group tasks into deliverables.",
    {
      project_id: z.string().describe("Project ID"),
      verbose: z.boolean().optional().describe("Return full details (default: false)"),
    },
    async ({ project_id, verbose }) => {
      const milestones = await client.listMilestones(project_id);
      const data = verbose
        ? milestones
        : milestones.map((m: Milestone) => ({ id: m.id, name: m.name, status: m.status, task_count: m.task_count, target_date: m.target_date }));
      return { content: [{ type: "text", text: formatResponse(data, verbose) }] };
    }
  );

  // --- create_milestone ---
  server.tool(
    "create_milestone",
    "Create a new milestone in a project. Milestones group tasks into deliverables with target dates.",
    {
      project_id: z.string().describe("Project ID"),
      name: z.string().describe("Milestone name"),
      description: z.string().optional().describe("Milestone description"),
      color: z.string().optional().describe("Hex color (default: #5e6ad2)"),
      target_date: z.string().optional().describe("Target date (YYYY-MM-DD)"),
      status: z.enum(["active", "completed", "cancelled"]).optional().describe("Status (default: active)"),
    },
    async ({ project_id, name, description, color, target_date, status }) => {
      const milestone = await client.createMilestone(project_id, { name, description, color, target_date, status });
      return { content: [{ type: "text", text: formatResponse(milestone) }] };
    }
  );

  // --- update_milestone ---
  server.tool(
    "update_milestone",
    "Update a milestone's name, description, color, target date, or status.",
    {
      milestone_id: z.string().describe("Milestone ID"),
      name: z.string().optional().describe("New name"),
      description: z.string().optional().describe("New description"),
      color: z.string().optional().describe("New hex color"),
      target_date: z.string().optional().describe("New target date (YYYY-MM-DD)"),
      status: z.enum(["active", "completed", "cancelled"]).optional().describe("New status"),
      sort_order: z.number().optional().describe("Display order"),
    },
    async ({ milestone_id, ...data }) => {
      const milestone = await client.updateMilestone(milestone_id, data);
      return { content: [{ type: "text", text: formatResponse(milestone) }] };
    }
  );

  // --- get_milestone_progress ---
  server.tool(
    "get_milestone_progress",
    "Get computed progress for a milestone: total/completed tasks, percentage, hours, by-assignee breakdown.",
    {
      milestone_id: z.string().describe("Milestone ID"),
      verbose: z.boolean().optional().describe("Pretty print JSON (default: false)"),
    },
    async ({ milestone_id, verbose }) => {
      const progress = await client.getMilestoneProgress(milestone_id);
      return { content: [{ type: "text", text: formatResponse(progress, verbose) }] };
    }
  );

  // --- add_dependency ---
  server.tool(
    "add_dependency",
    "Add a dependency between tasks. Task A depends on (is blocked by) task B. Cycle detection prevents circular dependencies.",
    {
      task_id: z.string().describe("Task ID (the task that depends on another)"),
      depends_on_id: z.number().describe("ID of the task it depends on (blocker)"),
      dependency_type: z.enum(["blocks", "related"]).optional().describe("Type of dependency (default: blocks)"),
    },
    async ({ task_id, depends_on_id, dependency_type }) => {
      const dep = await client.createDependency(task_id, { depends_on_id, dependency_type });
      return { content: [{ type: "text", text: formatResponse(dep) }] };
    }
  );

  // --- remove_dependency ---
  server.tool(
    "remove_dependency",
    "Remove a task dependency by its ID.",
    {
      dependency_id: z.string().describe("Dependency ID to remove"),
    },
    async ({ dependency_id }) => {
      await client.deleteDependency(dependency_id);
      return { content: [{ type: "text", text: "Dependency removed successfully" }] };
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

  // Extract default project scope from header or query param (supports comma-separated: "1,2,3")
  const rawProjectIds = (req.headers["x-project-id"] as string | undefined)
    ?? (req.query.project_id as string | undefined);
  const defaultProjectIds = rawProjectIds
    ? rawProjectIds.split(",").map(s => s.trim()).filter(Boolean)
    : undefined;

  // Create MCP server with authenticated client and cached user
  const server = createServer(client, cachedUser, defaultProjectIds);

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
