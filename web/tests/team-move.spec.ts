import { test, expect, type Page } from '@playwright/test';

/**
 * "Move to team" on Project settings (project owners only, including
 * co-owners with the Owner role), and the modern Select.
 *
 * Signup needs an invite code, so this spec runs against seeded accounts:
 *   E2E_OWNER_EMAIL     the project's recorded owner
 *   E2E_CO_OWNER_EMAIL  another member with the Owner role
 *   E2E_NON_OWNER_EMAIL a member without the Owner role (e.g. an editor)
 *   E2E_PASSWORD        password for both
 *   E2E_PROJECT_ID      the project both can open
 * It is skipped when they are not set.
 */
const owner = process.env.E2E_OWNER_EMAIL;
const coOwner = process.env.E2E_CO_OWNER_EMAIL;
const nonOwner = process.env.E2E_NON_OWNER_EMAIL;
const password = process.env.E2E_PASSWORD;
const projectId = process.env.E2E_PROJECT_ID;

test.skip(!owner || !coOwner || !nonOwner || !password || !projectId, 'set E2E_OWNER_EMAIL, E2E_CO_OWNER_EMAIL, E2E_NON_OWNER_EMAIL, E2E_PASSWORD, E2E_PROJECT_ID');

async function openTeamMembers(page: Page, email: string) {
  const res = await page.request.post('/api/auth/login', { data: { email, password } });
  expect(res.ok()).toBeTruthy();
  const { token } = await res.json();
  await page.addInitScript((t) => localStorage.setItem('auth_token', t), token);
  await page.goto(`/app/projects/${projectId}?tab=settings`);
  await expect(page.getByRole('heading', { name: 'Team Members' })).toBeVisible();
}

test('the owner gets the team picker and must confirm a move', async ({ page }) => {
  await openTeamMembers(page, owner!);
  expect(await page.locator('select').count()).toBe(0);

  const trigger = page.getByLabel('Move to');
  await trigger.click();
  const listbox = page.getByRole('listbox');
  await expect(listbox).toBeVisible();
  const current = listbox.getByRole('option', { selected: true });
  await expect(current).toContainText('Current');

  // Keyboard: move off the current team and pick it; a confirm dialog appears.
  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('Enter');
  const dialog = page.getByRole('alertdialog');
  await expect(dialog).toContainText('can be invited from now on');
  await dialog.getByRole('button', { name: 'Cancel' }).click();
  await expect(dialog).toBeHidden();
});

test('a co-owner with the Owner role also gets the team picker', async ({ page }) => {
  await openTeamMembers(page, coOwner!);
  await page.getByLabel('Move to').click();
  await expect(page.getByRole('listbox').getByRole('option', { selected: true })).toContainText('Current');
});

test('a non-owner sees the team read-only', async ({ page }) => {
  await openTeamMembers(page, nonOwner!);
  await expect(page.getByText('Team', { exact: true })).toBeVisible();
  await expect(page.getByLabel('Move to')).toHaveCount(0);
});

test('the API refuses a move by a non-owner', async ({ page }) => {
  const login = await page.request.post('/api/auth/login', { data: { email: nonOwner, password } });
  const { token } = await login.json();
  const res = await page.request.patch(`/api/projects/${projectId}`, {
    headers: { Authorization: `Bearer ${token}` },
    data: { team_id: 1 },
  });
  expect(res.status()).toBe(403);
  expect((await res.json()).error).toBe('only a project owner can move this project to another team');
});
