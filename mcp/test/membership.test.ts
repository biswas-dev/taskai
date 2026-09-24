import { test } from "node:test";
import assert from "node:assert/strict";

import type { ProjectMember, TeamMember, WikiSharing } from "../src/api.js";
import {
  MembershipClient,
  ToolInputError,
  addProjectMember,
  nextShareList,
  resolveMembers,
  updateWikiSharing,
} from "../src/membership.js";

const member = (user_id: number, email: string, role = "member"): ProjectMember => ({
  id: user_id * 10,
  project_id: 2,
  user_id,
  email,
  name: null,
  role,
  granted_by: 1,
  granted_at: "2026-01-01T00:00:00Z",
});

const teamMember = (user_id: number, email: string): TeamMember => ({
  id: user_id * 100,
  team_id: 7,
  user_id,
  email,
  role: "member",
  status: "active",
  joined_at: "2026-01-01T00:00:00Z",
});

/** In-memory TaskAI: one project (2) in team 7, one wiki page (50). */
function fakeClient(opts: {
  projectMembers?: ProjectMember[];
  teamMembers?: TeamMember[];
  registeredUsers?: Record<string, number>;
  sharing?: Partial<WikiSharing>;
  teamId?: number | null;
}) {
  const projectMembers = [...(opts.projectMembers ?? [])];
  const teamMembers = [...(opts.teamMembers ?? [])];
  const registered = opts.registeredUsers ?? {};
  let sharing: WikiSharing = {
    visibility: "project",
    can_manage: true,
    created_by: 1,
    shared_with: [],
    ...opts.sharing,
  };
  const calls: string[] = [];

  const client = {
    async getWikiPage(pageId: string) {
      return { id: pageId, project_id: 2 } as never;
    },
    async getWikiSharing() {
      return sharing;
    },
    async updateWikiSharing(_pageId: string, visibility: WikiSharing["visibility"], userIds: number[]) {
      calls.push(`updateWikiSharing ${visibility} [${userIds.join(",")}]`);
      sharing = {
        ...sharing,
        visibility,
        shared_with: userIds.map((id) => ({
          user_id: id,
          email: projectMembers.find((m) => m.user_id === id)?.email ?? "?",
        })),
      };
      return sharing;
    },
    async getProject(id: string) {
      return { id, name: "Elastio", team_id: opts.teamId === undefined ? 7 : opts.teamId } as never;
    },
    async listProjectMembers() {
      return projectMembers;
    },
    async addProjectMember(_projectId: string, email: string, role: string) {
      calls.push(`addProjectMember ${email} ${role}`);
      const tm = teamMembers.find((m) => m.email === email);
      if (!tm) throw new Error("TaskAI API error 400: User must be a member of this project's team");
      projectMembers.push(member(tm.user_id, tm.email, role));
      return { message: "Member added successfully", member_id: 1 };
    },
    async listTeamMembers() {
      return teamMembers;
    },
    async addTeamMember(_teamId: number, userId: number) {
      calls.push(`addTeamMember ${userId}`);
      const email = Object.keys(registered).find((e) => registered[e] === userId);
      if (!email) throw new Error("TaskAI API error 404: user not found");
      teamMembers.push(teamMember(userId, email));
      return { message: "member added" };
    },
    async inviteTeamMember(teamId: number, email: string) {
      calls.push(`inviteTeamMember ${email}`);
      const userId = registered[email];
      if (userId === undefined) {
        return { id: 1, team_id: teamId, team_name: "Elastio", invitee_email: email, status: "pending" };
      }
      teamMembers.push(teamMember(userId, email));
      return { id: 1, team_id: teamId, team_name: "Elastio", invitee_email: email, invitee_id: userId, status: "accepted" };
    },
  } satisfies MembershipClient;

  return { client, calls, getSharing: () => sharing };
}

test("resolveMembers matches emails case-insensitively and reports non-members", () => {
  const members = [member(3, "Amy@Example.com"), member(4, "bob@example.com")];
  const result = resolveMembers(members, [4, 99], [" amy@example.com ", "gary@example.com"]);
  assert.deepEqual(result.ids, [4, 3]);
  assert.deepEqual(result.unknownUserIds, [99]);
  assert.deepEqual(result.unknownEmails, ["gary@example.com"]);
});

test("nextShareList adds to or replaces the current list without duplicates", () => {
  assert.deepEqual(nextShareList([3, 4], [4, 5], "add"), [3, 4, 5]);
  assert.deepEqual(nextShareList([3, 4], [5, 5], "replace"), [5]);
});

test("update_wiki_sharing adds people by email and keeps existing shares", async () => {
  const { client, calls } = fakeClient({
    projectMembers: [member(3, "amy@example.com"), member(4, "bob@example.com")],
    sharing: { visibility: "restricted", shared_with: [{ user_id: 3, email: "amy@example.com" }] },
  });
  const result = await updateWikiSharing(client, { page_id: "50", visibility: "restricted", emails: ["BOB@example.com"] });
  assert.deepEqual(calls, ["updateWikiSharing restricted [3,4]"]);
  assert.deepEqual(result.shared_with.map((u) => u.user_id), [3, 4]);
});

test("update_wiki_sharing replace mode sets the whole list", async () => {
  const { client, calls } = fakeClient({
    projectMembers: [member(3, "amy@example.com"), member(4, "bob@example.com")],
    sharing: { visibility: "restricted", shared_with: [{ user_id: 3, email: "amy@example.com" }] },
  });
  await updateWikiSharing(client, { page_id: "50", visibility: "restricted", user_ids: [4], mode: "replace" });
  assert.deepEqual(calls, ["updateWikiSharing restricted [4]"]);
});

test("update_wiki_sharing without people keeps the list, unless replacing", async () => {
  const shared = { visibility: "restricted" as const, shared_with: [{ user_id: 3, email: "amy@example.com" }] };
  const keep = fakeClient({ sharing: shared });
  await updateWikiSharing(keep.client, { page_id: "50", visibility: "project" });
  assert.deepEqual(keep.calls, ["updateWikiSharing project [3]"]);

  const clear = fakeClient({ sharing: shared });
  await updateWikiSharing(clear.client, { page_id: "50", visibility: "restricted", mode: "replace" });
  assert.deepEqual(clear.calls, ["updateWikiSharing restricted []"]);
});

test("update_wiki_sharing fails clearly on a non-member and changes nothing", async () => {
  const { client, calls } = fakeClient({ projectMembers: [member(3, "amy@example.com")] });
  await assert.rejects(
    updateWikiSharing(client, { page_id: "50", visibility: "restricted", emails: ["amy@example.com", "gary@example.com"] }),
    (err: unknown) =>
      err instanceof ToolInputError && /Not members of project 2: gary@example.com/.test(err.message) && /add_project_member/.test(err.message),
  );
  assert.deepEqual(calls, []);
});

test("update_wiki_sharing refuses when the caller cannot manage the page", async () => {
  const { client, calls } = fakeClient({ sharing: { can_manage: false } });
  await assert.rejects(
    updateWikiSharing(client, { page_id: "50", visibility: "restricted", emails: ["amy@example.com"] }),
    ToolInputError,
  );
  assert.deepEqual(calls, []);
});

test("add_project_member adds a team member by email", async () => {
  const { client, calls } = fakeClient({ teamMembers: [teamMember(9, "gary@example.com")] });
  const result = await addProjectMember(client, { project_id: "2", email: "Gary@Example.com", role: "editor" });
  assert.equal(result.status, "added");
  assert.equal(result.added_to_team, false);
  assert.deepEqual(result.member, { user_id: 9, email: "gary@example.com", name: null, role: "editor" });
  // Uses the email as stored, since the API matches it exactly.
  assert.deepEqual(calls, ["addProjectMember gary@example.com editor"]);
});

test("add_project_member leaves an existing member alone", async () => {
  const { client, calls } = fakeClient({ projectMembers: [member(9, "gary@example.com", "viewer")] });
  const result = await addProjectMember(client, { project_id: "2", user_id: 9, role: "owner" });
  assert.equal(result.status, "already_member");
  assert.equal(result.member?.role, "viewer");
  assert.deepEqual(calls, []);
});

test("add_project_member refuses someone outside the team unless add_to_team is set", async () => {
  const { client, calls } = fakeClient({ registeredUsers: { "gary@example.com": 9 } });
  await assert.rejects(
    addProjectMember(client, { project_id: "2", email: "gary@example.com" }),
    (err: unknown) => err instanceof ToolInputError && /add_to_team=true/.test(err.message),
  );
  assert.deepEqual(calls, []);
});

test("add_project_member with add_to_team adds a registered user to the team, then the project", async () => {
  const byEmail = fakeClient({ registeredUsers: { "gary@example.com": 9 } });
  const r1 = await addProjectMember(byEmail.client, { project_id: "2", email: "gary@example.com", add_to_team: true });
  assert.equal(r1.status, "added");
  assert.equal(r1.added_to_team, true);
  assert.deepEqual(byEmail.calls, ["inviteTeamMember gary@example.com", "addProjectMember gary@example.com member"]);

  const byId = fakeClient({ registeredUsers: { "gary@example.com": 9 } });
  const r2 = await addProjectMember(byId.client, { project_id: "2", user_id: 9, add_to_team: true });
  assert.equal(r2.status, "added");
  assert.deepEqual(byId.calls, ["addTeamMember 9", "addProjectMember gary@example.com member"]);
});

test("add_project_member reports a pending signup invitation for someone with no account", async () => {
  const { client, calls } = fakeClient({});
  const result = await addProjectMember(client, { project_id: "2", email: "new@example.com", add_to_team: true });
  assert.equal(result.status, "team_invitation_pending");
  assert.match(result.message, /NOT in project 2 yet/);
  assert.deepEqual(calls, ["inviteTeamMember new@example.com"]);
});

test("add_project_member needs exactly one of email or user_id", async () => {
  const { client } = fakeClient({});
  await assert.rejects(addProjectMember(client, { project_id: "2" }), ToolInputError);
  await assert.rejects(addProjectMember(client, { project_id: "2", email: "a@b.co", user_id: 1 }), ToolInputError);
});
