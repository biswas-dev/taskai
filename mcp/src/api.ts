/**
 * TaskAI REST API client.
 * Wraps fetch calls with Authorization: ApiKey header.
 */

export interface Project {
  id: string;
  name: string;
  description: string;
  /** The team the project belongs to; only that team's members can join it. */
  team_id?: number | null;
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
  /** Parent page ID, or null for a top-level page. */
  parent_id: string | null;
  /** Sort order among sibling pages. */
  position: number;
  content?: string;
  created_by: string;
  created_at: string;
  updated_at: string;
  [key: string]: unknown;
}

export type WikiVisibility = "project" | "restricted";

export interface WikiShareUser {
  user_id: number;
  email: string;
  user_name?: string;
}

export interface WikiSharing {
  visibility: WikiVisibility;
  /** Whether the caller may change sharing (page creator or project owner). */
  can_manage: boolean;
  created_by: number;
  /** Explicit shares. The creator and project owner can always see the page and are not listed. */
  shared_with: WikiShareUser[];
  /** Set when a public read-only link is on; only returned to people who can manage the page. */
  public_token?: string;
}

export type ProjectRole = "viewer" | "member" | "editor" | "owner";

export interface ProjectMember {
  /** Membership row ID, not the user ID. */
  id: number;
  project_id: number;
  user_id: number;
  email: string;
  name?: string | null;
  role: string;
  granted_by: number;
  granted_at: string;
}

export interface TeamMember {
  id: number;
  team_id: number;
  user_id: number;
  user_name?: string;
  email: string;
  role: string;
  status: string;
  joined_at: string;
}

export interface TeamInvitation {
  id: number;
  team_id: number;
  team_name: string;
  invitee_email: string;
  invitee_id?: number;
  /** "accepted" when the user already existed and was added; "pending" when a signup email was sent. */
  status: string;
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

export type AnnotationColor = "yellow" | "blue" | "green" | "red";

export interface WikiAnnotationComment {
  id: number;
  annotation_id: number;
  content: string;
  author_id: number;
  author_name?: string;
  parent_comment_id?: number | null;
  created_at: string;
  updated_at: string;
  [key: string]: unknown;
}

export interface WikiAnnotation {
  id: number;
  page_id: number;
  start_offset: number;
  end_offset: number;
  selected_text: string;
  color: AnnotationColor;
  resolved: boolean;
  created_by: number;
  creator_name?: string;
  created_at: string;
  updated_at: string;
  comments?: WikiAnnotationComment[];
  [key: string]: unknown;
}

export interface Drawing {
  id: string;
  title: string;
  scene: unknown;
}

export interface Milestone {
  id: number;
  project_id: number;
  name: string;
  description?: string;
  color: string;
  target_date?: string;
  status: string;
  sort_order: number;
  task_count?: number;
  created_at: string;
  updated_at: string;
}

export interface MilestoneProgress {
  milestone_id: number;
  milestone_name: string;
  total_tasks: number;
  completed_tasks: number;
  percentage: number;
  by_status: Record<string, number>;
  estimated_hours: number;
  actual_hours: number;
}

export interface TaskDependency {
  id: number;
  task_id: number;
  depends_on_id: number;
  dependency_type: string;
  created_at: string;
}

export interface TaskDependencies {
  blocked_by: TaskDependency[];
  blocks: TaskDependency[];
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

/**
 * GET /api/projects returns a bare array of projects. Normalise it (and the
 * `{ projects, total }` envelope, should the API ever paginate) to one shape
 * so callers never map over an undefined field.
 */
export function normalizeProjectList(body: unknown): { projects: Project[]; total: number } {
  if (Array.isArray(body)) {
    return { projects: body as Project[], total: body.length };
  }
  const envelope = (body ?? {}) as { projects?: unknown; total?: unknown };
  const projects = Array.isArray(envelope.projects) ? (envelope.projects as Project[]) : [];
  const total = typeof envelope.total === "number" ? envelope.total : projects.length;
  return { projects, total };
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
    const body = await this.request<unknown>(`/api/projects?page=${page}&limit=${limit}`);
    return normalizeProjectList(body);
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

  async createSwimLane(
    projectId: string,
    data: { name: string; status_category: string; color?: string; position?: number }
  ): Promise<SwimLane> {
    return this.request<SwimLane>(`/api/projects/${encodeURIComponent(projectId)}/swim-lanes`, {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async updateSwimLane(
    swimLaneId: number,
    data: { name?: string; color?: string; position?: number; status_category?: string }
  ): Promise<SwimLane> {
    return this.request<SwimLane>(`/api/swim-lanes/${swimLaneId}`, {
      method: "PATCH",
      body: JSON.stringify(data),
    });
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
    project_ids?: string[];
    limit?: number;
    recency_days?: number;
    mode?: string;
  }): Promise<{ results: WikiBlock[]; total: number }> {
    return this.request("/api/wiki/search", {
      method: "POST",
      body: JSON.stringify(params),
    });
  }

  async reindexWiki(): Promise<{ status: string; message: string }> {
    return this.request("/api/wiki/reindex", {
      method: "POST",
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

  async listWikiAnnotations(pageId: string): Promise<WikiAnnotation[]> {
    return this.request<WikiAnnotation[]>(`/api/wiki/pages/${encodeURIComponent(pageId)}/annotations`);
  }

  async createWikiAnnotation(
    pageId: string,
    data: {
      start_offset: number;
      end_offset: number;
      selected_text: string;
      color: AnnotationColor;
      comment?: string;
    }
  ): Promise<WikiAnnotation> {
    return this.request<WikiAnnotation>(`/api/wiki/pages/${encodeURIComponent(pageId)}/annotations`, {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async updateWikiAnnotation(
    annotationId: string,
    data: { color?: AnnotationColor; resolved?: boolean }
  ): Promise<WikiAnnotation> {
    return this.request<WikiAnnotation>(`/api/wiki/annotations/${encodeURIComponent(annotationId)}`, {
      method: "PATCH",
      body: JSON.stringify(data),
    });
  }

  async deleteWikiAnnotation(annotationId: string): Promise<void> {
    await this.request(`/api/wiki/annotations/${encodeURIComponent(annotationId)}`, {
      method: "DELETE",
    });
  }

  async createWikiAnnotationComment(
    annotationId: string,
    data: { content: string; parent_comment_id?: number }
  ): Promise<WikiAnnotationComment> {
    return this.request<WikiAnnotationComment>(`/api/wiki/annotations/${encodeURIComponent(annotationId)}/comments`, {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async updateWikiAnnotationComment(commentId: string, content: string): Promise<WikiAnnotationComment> {
    return this.request<WikiAnnotationComment>(`/api/wiki/annotation-comments/${encodeURIComponent(commentId)}`, {
      method: "PATCH",
      body: JSON.stringify({ content }),
    });
  }

  async deleteWikiAnnotationComment(commentId: string): Promise<void> {
    await this.request(`/api/wiki/annotation-comments/${encodeURIComponent(commentId)}`, {
      method: "DELETE",
    });
  }

  async getWikiPagePdf(pageId: string): Promise<{ data: string; filename: string }> {
    const headers: Record<string, string> = {
      Authorization: `ApiKey ${this.apiKey}`,
      "Content-Type": "application/json",
    };
    if (this.agentName) headers["X-Agent-Name"] = this.agentName;

    // Start async job
    const startRes = await fetch(`${this.baseURL}/api/wiki/pages/${encodeURIComponent(pageId)}/pdf`, {
      method: "POST",
      headers,
    });
    if (!startRes.ok) throw new Error(`PDF start failed ${startRes.status}: ${await startRes.text()}`);
    const { job_id } = (await startRes.json()) as { job_id: string };

    // Poll until done (max 2 min)
    const pollHeaders: Record<string, string> = { Authorization: `ApiKey ${this.apiKey}` };
    const deadline = Date.now() + 120_000;
    while (Date.now() < deadline) {
      await new Promise((r) => setTimeout(r, 2000));
      const pollRes = await fetch(
        `${this.baseURL}/api/wiki/pages/${encodeURIComponent(pageId)}/pdf/${job_id}`,
        { headers: pollHeaders },
      );
      if (!pollRes.ok) throw new Error(`PDF poll failed ${pollRes.status}`);

      const ct = pollRes.headers.get("Content-Type") || "";
      if (ct.includes("application/pdf")) {
        const buf = await pollRes.arrayBuffer();
        const data = Buffer.from(buf).toString("base64");
        const cd = pollRes.headers.get("Content-Disposition") || "";
        const filename = cd.match(/filename="(.+)"/)?.[1] || "wiki-page.pdf";
        return { data, filename };
      }

      const status = (await pollRes.json()) as { status: string; error?: string };
      if (status.status === "failed") throw new Error(status.error || "PDF generation failed");
    }
    throw new Error("PDF generation timed out");
  }

  async getWikiPageMarkdown(pageId: string): Promise<{ content: string; filename: string }> {
    const url = `${this.baseURL}/api/wiki/pages/${encodeURIComponent(pageId)}/markdown`;
    const headers: Record<string, string> = { Authorization: `ApiKey ${this.apiKey}` };
    if (this.agentName) headers["X-Agent-Name"] = this.agentName;
    const res = await fetch(url, { headers });
    if (!res.ok) {
      const text = await res.text();
      throw new Error(`API error ${res.status}: ${text}`);
    }
    const content = await res.text();
    const cd = res.headers.get("Content-Disposition") || "";
    const filename = cd.match(/filename="(.+)"/)?.[1] || "wiki-page.md";
    return { content, filename };
  }

  async createWikiPage(projectId: string, title: string, parentId?: string): Promise<WikiPage> {
    const body: { title: string; parent_id?: number } = { title };
    if (parentId !== undefined && parentId !== "") body.parent_id = Number(parentId);
    return this.request<WikiPage>(`/api/projects/${encodeURIComponent(projectId)}/wiki/pages`, {
      method: "POST",
      body: JSON.stringify(body),
    });
  }

  async updateWikiPageContent(pageId: string, content: string): Promise<WikiPage> {
    return this.request<WikiPage>(`/api/wiki/pages/${encodeURIComponent(pageId)}/content`, {
      method: "PUT",
      body: JSON.stringify({ content, manual_save: true }),
    });
  }

  async updateWikiPage(
    pageId: string,
    data: { title?: string; parent_id?: number | null; position?: number },
  ): Promise<WikiPage> {
    return this.request<WikiPage>(`/api/wiki/pages/${encodeURIComponent(pageId)}`, {
      method: "PATCH",
      body: JSON.stringify(data),
    });
  }

  // --- Wiki sharing endpoints ---
  async getWikiSharing(pageId: string): Promise<WikiSharing> {
    return this.request<WikiSharing>(`/api/wiki/pages/${encodeURIComponent(pageId)}/sharing`);
  }

  /** Sets visibility and the FULL share list (the API replaces the list). */
  async updateWikiSharing(pageId: string, visibility: WikiVisibility, userIds: number[]): Promise<WikiSharing> {
    return this.request<WikiSharing>(`/api/wiki/pages/${encodeURIComponent(pageId)}/sharing`, {
      method: "PUT",
      body: JSON.stringify({ visibility, user_ids: userIds }),
    });
  }

  // --- Project member endpoints ---
  async listProjectMembers(projectId: string): Promise<ProjectMember[]> {
    return this.request<ProjectMember[]>(`/api/projects/${encodeURIComponent(projectId)}/members`);
  }

  /** Adds an existing user directly (no acceptance step). They must already be in the project's team. */
  async addProjectMember(projectId: string, email: string, role: ProjectRole): Promise<{ message: string; member_id: number }> {
    return this.request(`/api/projects/${encodeURIComponent(projectId)}/members`, {
      method: "POST",
      body: JSON.stringify({ email, role }),
    });
  }

  // --- Team member endpoints ---
  async listTeamMembers(teamId: number): Promise<TeamMember[]> {
    return this.request<TeamMember[]>(`/api/teams/${teamId}/members`);
  }

  /** Adds an existing user to a team by user ID. */
  async addTeamMember(teamId: number, userId: number): Promise<{ message: string }> {
    return this.request(`/api/teams/${teamId}/members`, {
      method: "POST",
      body: JSON.stringify({ user_id: userId }),
    });
  }

  /**
   * Invites an email to a team. A registered user is added at once (status
   * "accepted"); anyone else gets a signup email and a "pending" invitation.
   */
  async inviteTeamMember(teamId: number, email: string): Promise<TeamInvitation> {
    return this.request<TeamInvitation>(`/api/teams/${teamId}/invite`, {
      method: "POST",
      body: JSON.stringify({ email }),
    });
  }

  // Version/health methods
  // --- Milestone endpoints ---
  async listMilestones(projectId: string): Promise<Milestone[]> {
    return this.request<Milestone[]>(`/api/projects/${encodeURIComponent(projectId)}/milestones`);
  }

  async createMilestone(
    projectId: string,
    data: { name: string; description?: string; color?: string; target_date?: string; status?: string }
  ): Promise<Milestone> {
    return this.request<Milestone>(`/api/projects/${encodeURIComponent(projectId)}/milestones`, {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async updateMilestone(
    milestoneId: string,
    data: { name?: string; description?: string; color?: string; target_date?: string; status?: string; sort_order?: number }
  ): Promise<Milestone> {
    return this.request<Milestone>(`/api/milestones/${encodeURIComponent(milestoneId)}`, {
      method: "PATCH",
      body: JSON.stringify(data),
    });
  }

  async deleteMilestone(milestoneId: string): Promise<void> {
    await this.request(`/api/milestones/${encodeURIComponent(milestoneId)}`, { method: "DELETE" });
  }

  async getMilestoneProgress(milestoneId: string): Promise<MilestoneProgress> {
    return this.request<MilestoneProgress>(`/api/milestones/${encodeURIComponent(milestoneId)}/progress`);
  }

  // --- Task dependency endpoints ---
  async listDependencies(taskId: string): Promise<TaskDependencies> {
    return this.request<TaskDependencies>(`/api/tasks/${encodeURIComponent(taskId)}/dependencies`);
  }

  async createDependency(
    taskId: string,
    data: { depends_on_id: number; dependency_type?: string }
  ): Promise<TaskDependency> {
    return this.request<TaskDependency>(`/api/tasks/${encodeURIComponent(taskId)}/dependencies`, {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  async deleteDependency(dependencyId: string): Promise<void> {
    await this.request(`/api/task-dependencies/${encodeURIComponent(dependencyId)}`, { method: "DELETE" });
  }

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
