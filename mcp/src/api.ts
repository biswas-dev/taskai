/**
 * TaskAI REST API client.
 * Wraps fetch calls with Authorization: ApiKey header.
 */

export interface Project {
  id: string;
  name: string;
  description: string;
  created_at: string;
  updated_at: string;
  [key: string]: unknown;
}

export interface Task {
  id: string;
  project_id: string;
  task_number: number;
  title: string;
  description: string;
  status: string;
  priority: string;
  assigned_to: string;
  created_at: string;
  updated_at: string;
  [key: string]: unknown;
}

export interface SwimLane {
  id: number;
  project_id: number;
  name: string;
  color: string;
  position: number;
  status_category: string;
  created_at: string;
  updated_at: string;
  [key: string]: unknown;
}

export interface Comment {
  id: string;
  task_id: string;
  content: string;
  author_id: string;
  created_at: string;
  [key: string]: unknown;
}

export interface User {
  id: string;
  email: string;
  is_admin: boolean;
  [key: string]: unknown;
}

export interface WikiPage {
  id: string;
  project_id: string;
  title: string;
  slug: string;
  content?: string;
  created_by: string;
  created_at: string;
  updated_at: string;
  [key: string]: unknown;
}

export interface ProjectDrawing {
  id: string;
  project_id: string;
  draw_id: string;
  created_by: string;
  created_at: string;
}

export interface WikiPageContent {
  page_id: number;
  content: string;
  updated_at: string;
}

export interface Drawing {
  id: string;
  title: string;
  scene: unknown;
}

export interface WikiBlock {
  page_id: string;
  page_title: string;
  page_slug: string;
  block_id: string;
  block_type: string;
  headings_path: string;
  snippet: string;
  rank?: number;
  [key: string]: unknown;
}

export class TaskAIClient {
  private baseURL: string;
  private apiKey: string;
  public agentName?: string;

  constructor(baseURL: string, apiKey: string) {
    // Strip trailing slash
    this.baseURL = baseURL.replace(/\/+$/, "");
    this.apiKey = apiKey;
  }

  private async request<T>(path: string, options: RequestInit = {}): Promise<T> {
    const url = `${this.baseURL}${path}`;
    const headers: Record<string, string> = {
      "Content-Type": "application/json",
      Authorization: `ApiKey ${this.apiKey}`,
    };
    if (this.agentName) {
      headers["X-Agent-Name"] = this.agentName;
    }
    const res = await fetch(url, {
      ...options,
      headers: {
        ...headers,
        ...options.headers as Record<string, string>,
      },
    });

    if (!res.ok) {
      const body = await res.text();
      throw new Error(`TaskAI API error ${res.status}: ${body}`);
    }

    return res.json() as Promise<T>;
  }

  async getMe(): Promise<User> {
    return this.request<User>("/api/me");
  }

  async listProjects(page = 1, limit = 20): Promise<{ projects: Project[]; total: number }> {
    return this.request(`/api/projects?page=${page}&limit=${limit}`);
  }

  async getProject(id: string): Promise<Project> {
    return this.request<Project>(`/api/projects/${encodeURIComponent(id)}`);
  }

  async listTasks(
    projectId: string,
    params?: { query?: string; status?: string; page?: number; limit?: number }
  ): Promise<{ tasks: Task[]; total: number }> {
    const qs = new URLSearchParams();
    if (params?.query) qs.set("query", params.query);
    if (params?.status) qs.set("status", params.status);
    if (params?.page) qs.set("page", String(params.page));
    if (params?.limit) qs.set("limit", String(params.limit));
    const suffix = qs.toString() ? `?${qs}` : "";
    return this.request(`/api/projects/${encodeURIComponent(projectId)}/tasks${suffix}`);
  }

  async getTaskByNumber(projectId: string, taskNumber: number): Promise<Task> {
    return this.request<Task>(`/api/projects/${encodeURIComponent(projectId)}/tasks/${taskNumber}`);
  }

  async listSwimLanes(projectId: string): Promise<SwimLane[]> {
    return this.request<SwimLane[]>(`/api/projects/${encodeURIComponent(projectId)}/swim-lanes`);
  }

  async createTask(
    projectId: string,
    data: { title: string; description?: string; status?: string; priority?: string; assigned_to?: string; swim_lane_id?: number }
  ): Promise<Task> {
    return this.request<Task>(`/api/projects/${encodeURIComponent(projectId)}/tasks`, {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async updateTask(
    taskId: string,
    data: { title?: string; description?: string; status?: string; priority?: string; assigned_to?: string; swim_lane_id?: number }
  ): Promise<Task> {
    return this.request<Task>(`/api/tasks/${encodeURIComponent(taskId)}`, {
      method: "PATCH",
      body: JSON.stringify(data),
    });
  }

  async listComments(taskId: string): Promise<{ comments: Comment[] }> {
    return this.request(`/api/tasks/${encodeURIComponent(taskId)}/comments`);
  }

  async addComment(taskId: string, content: string): Promise<Comment> {
    return this.request<Comment>(`/api/tasks/${encodeURIComponent(taskId)}/comments`, {
      method: "POST",
      body: JSON.stringify({ comment: content }),
    });
  }

  async updateComment(commentId: string, content: string): Promise<Comment> {
    return this.request<Comment>(`/api/comments/${encodeURIComponent(commentId)}`, {
      method: "PATCH",
      body: JSON.stringify({ comment: content }),
    });
  }

  async deleteComment(commentId: string): Promise<{ id: number; deleted: boolean }> {
    return this.request(`/api/comments/${encodeURIComponent(commentId)}`, {
      method: "DELETE",
    });
  }

  // Wiki methods
  async searchWiki(params: {
    query: string;
    project_id?: string;
    limit?: number;
    recency_days?: number;
  }): Promise<{ results: WikiBlock[]; total: number }> {
    return this.request("/api/wiki/search", {
      method: "POST",
      body: JSON.stringify(params),
    });
  }

  async listWikiPages(projectId: string): Promise<WikiPage[]> {
    return this.request<WikiPage[]>(`/api/projects/${encodeURIComponent(projectId)}/wiki/pages`);
  }

  async getWikiPage(pageId: string): Promise<WikiPage> {
    return this.request<WikiPage>(`/api/wiki/pages/${encodeURIComponent(pageId)}`);
  }

  async autocompletePages(query: string, projectId?: string, limit = 10): Promise<Array<{ id: string; title: string; slug: string }>> {
    const qs = new URLSearchParams({ query, limit: String(limit) });
    if (projectId) qs.set("project_id", projectId);
    return this.request(`/api/wiki/autocomplete?${qs}`);
  }

  async listProjectDrawings(projectId: string): Promise<ProjectDrawing[]> {
    return this.request<ProjectDrawing[]>(`/api/projects/${encodeURIComponent(projectId)}/drawings`);
  }

  async createDrawing(
    projectId: string,
    opts?: { title?: string; scene?: unknown }
  ): Promise<{ draw_id: string; edit_url: string; view_url: string; shortcode: string }> {
    // go-draw /draw/api/new does not require auth — call without Authorization header
    const url = `${this.baseURL}/draw/api/new`;
    const body = opts ? JSON.stringify({ title: opts.title, scene: opts.scene }) : undefined;
    const res = await fetch(url, {
      method: "POST",
      headers: body ? { "Content-Type": "application/json" } : undefined,
      body,
    });
    if (!res.ok) {
      const text = await res.text();
      throw new Error(`go-draw API error ${res.status}: ${text}`);
    }
    const draw = await res.json() as { id: string; edit_url: string; view_url: string };

    // Register with project
    await this.request(`/api/projects/${encodeURIComponent(projectId)}/drawings`, {
      method: "POST",
      body: JSON.stringify({ draw_id: draw.id }),
    });

    return {
      draw_id: draw.id,
      edit_url: draw.edit_url,
      view_url: draw.view_url,
      shortcode: `[draw:${draw.id}:edit:m]`,
    };
  }

  async saveDrawing(drawId: string, title: string, scene: unknown): Promise<{ ok: boolean; id: string }> {
    const url = `${this.baseURL}/draw/${encodeURIComponent(drawId)}/save`;
    const res = await fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ title, scene }),
    });
    if (!res.ok) {
      const text = await res.text();
      throw new Error(`go-draw save error ${res.status}: ${text}`);
    }
    return res.json() as Promise<{ ok: boolean; id: string }>;
  }

  async getDrawing(drawId: string): Promise<Drawing> {
    const url = `${this.baseURL}/draw/${encodeURIComponent(drawId)}/data`;
    const res = await fetch(url);
    if (!res.ok) {
      const text = await res.text();
      throw new Error(`go-draw data error ${res.status}: ${text}`);
    }
    return res.json() as Promise<Drawing>;
  }

  async getWikiPageContent(pageId: string): Promise<WikiPageContent> {
    return this.request<WikiPageContent>(`/api/wiki/pages/${encodeURIComponent(pageId)}/content`);
  }

  async createWikiPage(projectId: string, title: string): Promise<WikiPage> {
    return this.request<WikiPage>(`/api/projects/${encodeURIComponent(projectId)}/wiki/pages`, {
      method: "POST",
      body: JSON.stringify({ title }),
    });
  }

  async updateWikiPageContent(pageId: string, content: string): Promise<WikiPage> {
    return this.request<WikiPage>(`/api/wiki/pages/${encodeURIComponent(pageId)}/content`, {
      method: "PUT",
      body: JSON.stringify({ content, manual_save: true }),
    });
  }

  // Version/health methods
  async getVersion(): Promise<{
    version: string;
    git_commit: string;
    build_time: string;
    go_version: string;
    platform: string;
    server_time: string;
    db_version: number;
    environment: string;
  }> {
    return this.request("/api/version");
  }

  async healthCheck(): Promise<{ status: string; database?: string }> {
    return this.request("/healthz");
  }
}
