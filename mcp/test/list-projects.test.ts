import { test, afterEach } from "node:test";
import assert from "node:assert/strict";

import { TaskAIClient, normalizeProjectList, type Project } from "../src/api.js";

const project = (id: string, name: string): Project => ({
  id,
  name,
  description: "",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
});

const realFetch = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = realFetch;
});

test("normalizeProjectList accepts the bare array the API returns", () => {
  const out = normalizeProjectList([project("1", "A"), project("2", "B")]);
  assert.equal(out.total, 2);
  assert.deepEqual(
    out.projects.map((p) => p.name),
    ["A", "B"],
  );
});

test("normalizeProjectList accepts a { projects, total } envelope", () => {
  const out = normalizeProjectList({ projects: [project("1", "A")], total: 9 });
  assert.equal(out.projects.length, 1);
  assert.equal(out.total, 9);
});

test("normalizeProjectList never yields an undefined projects field", () => {
  for (const body of [null, undefined, {}, { total: 3 }]) {
    const out = normalizeProjectList(body);
    assert.deepEqual(out.projects, []);
  }
});

test("listProjects result can be mapped when the API returns an array", async () => {
  globalThis.fetch = (async () =>
    new Response(JSON.stringify([project("1", "A"), project("2", "B")]), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    })) as typeof fetch;

  const client = new TaskAIClient("http://taskai.test/", "key");
  const result = await client.listProjects();
  assert.deepEqual(
    result.projects.map((p) => ({ id: p.id, name: p.name })),
    [
      { id: "1", name: "A" },
      { id: "2", name: "B" },
    ],
  );
  assert.equal(result.total, 2);
});
