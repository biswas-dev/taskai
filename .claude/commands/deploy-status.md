Show last deployment time per environment.

## Usage
```
/deploy-status           # show all environments
/deploy-status staging   # show staging only
/deploy-status uat       # show UAT only
/deploy-status prod      # show production only
```

## Steps

Parse `$ARGUMENTS` (trim whitespace, lowercase). If empty, show all three environments. Otherwise show only the requested one.

For each environment to display, run:

```bash
gh run list \
  --workflow="<workflow-name>" \
  --limit=1 \
  --json databaseId,displayTitle,status,conclusion,createdAt,updatedAt
```

Workflow names:
- staging  → `"Deploy to Staging"`
- uat      → `"Deploy to UAT"`
- prod / production → `"Deploy to Production"`

For each result, extract:
- `updatedAt` — when the run finished (convert to local time for readability)
- `conclusion` — success / failure / cancelled
- `displayTitle` — the commit message
- `databaseId` — use to build the run URL: `https://github.com/anchoo2kewl/taskai/actions/runs/<databaseId>`

## Output format

Print a table like:

```
| Environment | Last Deploy         | Status  | Commit                                         | Run |
|-------------|---------------------|---------|------------------------------------------------|-----|
| Staging     | 2026-03-05 22:05 UTC | ✅ success | fix(github): always fetch comments for tasks... | #22738841551 |
| UAT         | 2026-03-04 14:32 UTC | ✅ success | feat: knowledge graph linking                  | #22700001234 |
| Production  | 2026-03-04 15:01 UTC | ✅ success | feat: knowledge graph linking                  | #22700009999 |
```

Status icons:
- `success` → ✅
- `failure` → ❌
- `cancelled` → ⚠️
- `in_progress` / `queued` → 🔄

If a workflow has never run, show `—` in the row.

URLs:
- Staging:    https://staging.taskai.cc
- UAT:        https://uat.taskai.cc
- Production: https://taskai.cc

Include the env URL beneath or alongside the table.
Monitor all runs: https://github.com/anchoo2kewl/taskai/actions
