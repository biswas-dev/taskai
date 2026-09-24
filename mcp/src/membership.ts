/**
 * Wiki sharing and project membership logic behind the MCP tools.
 *
 * TaskAI's access model is team → project → wiki page:
 *   - every project belongs to one team, and only that team's active members
 *     can be added to the project;
 *   - a restricted wiki page can only be shared with members of its project.
 * These helpers resolve emails to user IDs against those lists and turn the
 * API's plain errors into messages that say what to do next.
 */
import type {
  TaskAIClient,
  ProjectMember,
  ProjectRole,
  TeamMember,
  WikiSharing,
  WikiVisibility,
} from "./api.js";

/** The client calls these flows need, so tests can pass a fake. */
export type MembershipClient = Pick<
  TaskAIClient,
  | "getWikiPage"
  | "getWikiSharing"
  | "updateWikiSharing"
  | "getProject"
  | "listProjectMembers"
  | "addProjectMember"
  | "listTeamMembers"
  | "addTeamMember"
  | "inviteTeamMember"
>;

/** A failure the caller can act on; the message is shown to the AI as-is. */
export class ToolInputError extends Error {}

export type ShareMode = "add" | "replace";

export function normalizeEmail(email: string): string {
  return email.trim().toLowerCase();
}

/**
 * Maps requested user IDs and emails to project members. Anything that is not
 * a member of the project is reported rather than dropped.
 */
export function resolveMembers(
  members: ProjectMember[],
  userIds: number[] = [],
  emails: string[] = [],
): { ids: number[]; unknownUserIds: number[]; unknownEmails: string[] } {
  const byId = new Set(members.map((m) => Number(m.user_id)));
  const byEmail = new Map(members.map((m) => [normalizeEmail(m.email), Number(m.user_id)]));

  const ids: number[] = [];
  const unknownUserIds: number[] = [];
  const unknownEmails: string[] = [];
  for (const id of userIds) {
    if (byId.has(id)) ids.push(id);
    else unknownUserIds.push(id);
  }
  for (const email of emails) {
    const id = byEmail.get(normalizeEmail(email));
    if (id !== undefined) ids.push(id);
    else unknownEmails.push(email.trim());
  }
  return { ids: [...new Set(ids)], unknownUserIds, unknownEmails };
}

/** The full share list to send: the API always replaces the whole list. */
export function nextShareList(current: number[], requested: number[], mode: ShareMode): number[] {
  return mode === "replace" ? [...new Set(requested)] : [...new Set([...current, ...requested])];
}

/**
 * Sets a page's visibility and share list. With mode "add" the requested
 * people are added to whoever already has access; with "replace" they become
 * the whole list. Omitting both user_ids and emails keeps the current list.
 */
export async function updateWikiSharing(
  client: MembershipClient,
  args: { page_id: string; visibility: WikiVisibility; user_ids?: number[]; emails?: string[]; mode?: ShareMode },
): Promise<WikiSharing> {
  const current = await client.getWikiSharing(args.page_id);
  if (!current.can_manage) {
    throw new ToolInputError(
      "Only the page's creator or the project owner can change its sharing; you can view it but not change it.",
    );
  }
  const currentIds = current.shared_with.map((u) => Number(u.user_id));
  const requestedAny = (args.user_ids?.length ?? 0) > 0 || (args.emails?.length ?? 0) > 0;
  if (!requestedAny) {
    if (args.mode === "replace") {
      return client.updateWikiSharing(args.page_id, args.visibility, []);
    }
    return client.updateWikiSharing(args.page_id, args.visibility, currentIds);
  }

  const page = await client.getWikiPage(args.page_id);
  const projectId = String(page.project_id);
  const members = await client.listProjectMembers(projectId);
  const { ids, unknownUserIds, unknownEmails } = resolveMembers(members, args.user_ids, args.emails);
  if (unknownEmails.length || unknownUserIds.length) {
    const who = [...unknownEmails, ...unknownUserIds.map((id) => `user ${id}`)].join(", ");
    throw new ToolInputError(
      `Not members of project ${projectId}: ${who}. A wiki page can only be shared with members of its project. ` +
        `Check spelling with list_project_members, or add them with add_project_member first. Nothing was changed.`,
    );
  }
  return client.updateWikiSharing(args.page_id, args.visibility, nextShareList(currentIds, ids, args.mode ?? "add"));
}

export interface AddProjectMemberResult {
  status: "added" | "already_member" | "team_invitation_pending";
  message: string;
  added_to_team?: boolean;
  member?: { user_id: number; email: string; name: string | null; role: string };
}

function summarizeMember(m: ProjectMember) {
  return { user_id: Number(m.user_id), email: m.email, name: m.name ?? null, role: m.role };
}

/**
 * Adds a user to a project, directly (no acceptance step). The user must be an
 * active member of the project's team; with addToTeam they are added to the
 * team first. A user without a TaskAI account can only be invited to the team
 * by email, and must sign up before they can join the project.
 */
export async function addProjectMember(
  client: MembershipClient,
  args: { project_id: string; email?: string; user_id?: number; role?: ProjectRole; add_to_team?: boolean },
): Promise<AddProjectMemberResult> {
  const hasEmail = !!args.email?.trim();
  if (hasEmail === (args.user_id !== undefined)) {
    throw new ToolInputError("Pass exactly one of email or user_id.");
  }
  const role = args.role ?? "member";
  const email = args.email?.trim();

  const findInProject = (list: ProjectMember[]) =>
    list.find((m) => (email ? normalizeEmail(m.email) === normalizeEmail(email) : Number(m.user_id) === args.user_id));

  const existing = findInProject(await client.listProjectMembers(args.project_id));
  if (existing) {
    return {
      status: "already_member",
      message: `Already a member of project ${args.project_id} with role '${existing.role}'. Role was not changed.`,
      member: summarizeMember(existing),
    };
  }

  // The API only lets a project's team members join it, and its add endpoint
  // takes an email, so find the user in the team (by email or ID) first.
  const project = await client.getProject(args.project_id);
  const teamId = project.team_id ?? undefined;
  let addedToTeam = false;
  let projectEmail = email;

  if (teamId !== undefined) {
    const findInTeam = (list: TeamMember[]) =>
      list.find((m) => (email ? normalizeEmail(m.email) === normalizeEmail(email) : Number(m.user_id) === args.user_id));

    let teamMember = findInTeam(await client.listTeamMembers(teamId));
    if (!teamMember) {
      const who = email ?? `user ${args.user_id}`;
      if (!args.add_to_team) {
        throw new ToolInputError(
          `${who} is not in team ${teamId}, which project ${args.project_id} belongs to. Only team members can join a project. ` +
            `Call again with add_to_team=true to add them to the team first (this also gives them access to join the team's other projects). Nothing was changed.`,
        );
      }
      if (email) {
        const invitation = await client.inviteTeamMember(teamId, email);
        if (invitation.status !== "accepted") {
          return {
            status: "team_invitation_pending",
            message:
              `${email} has no TaskAI account, so they were emailed an invitation to sign up and join team ${teamId}. ` +
              `They are NOT in project ${args.project_id} yet: once they have signed up, call add_project_member again.`,
          };
        }
      } else {
        await client.addTeamMember(teamId, args.user_id as number);
      }
      addedToTeam = true;
      teamMember = findInTeam(await client.listTeamMembers(teamId));
      if (!teamMember) {
        throw new Error(`Added ${who} to team ${teamId} but could not find them in its member list.`);
      }
    }
    projectEmail = teamMember.email;
  } else if (!projectEmail) {
    throw new ToolInputError(`Project ${args.project_id} has no team, so the user can only be added by email.`);
  }

  await client.addProjectMember(args.project_id, projectEmail as string, role);
  const added = findInProject(await client.listProjectMembers(args.project_id));
  return {
    status: "added",
    message:
      `Added to project ${args.project_id} as '${role}'` +
      (addedToTeam ? ` (and added to team ${teamId} first)` : "") +
      ". Restricted wiki pages in the project can now be shared with them.",
    added_to_team: addedToTeam,
    member: added ? summarizeMember(added) : undefined,
  };
}
