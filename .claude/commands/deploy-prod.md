Promote TaskAI all the way to production: staging → UAT → production.

## Usage
```
/deploy-prod   # promotes current staging through UAT and into production
```

No arguments needed. This command always promotes from staging → UAT → production in sequence.

## Rules
- Staging must already be deployed and healthy before running this
- UAT is promoted from staging (main branch), never developed on directly
- Production is promoted after UAT succeeds
- Never manually SSH to servers — always use GitHub workflows
- If any step fails, stop and report — do not proceed to the next environment

## Steps

### 1. Verify staging is healthy
Run:
```bash
gh run list --workflow="Deploy to Staging" --limit=1 --json status,conclusion,updatedAt
```
If the last staging run is not `conclusion: success`, abort and tell the user to fix staging first.

### 2. Promote staging → UAT
```bash
./script/server promote staging uat
```
Then wait for the UAT workflow to complete:
```bash
gh run list --workflow="Deploy to UAT" --limit=1 --json databaseId,status,conclusion,updatedAt
```
Poll every 15 seconds (up to 5 minutes) until `status` is `completed`. If `conclusion` is not `success`, abort and report the failure with the run URL.

### 3. Promote staging → Production
```bash
./script/server promote staging prod
```
Then wait for the production workflow to complete:
```bash
gh run list --workflow="Deploy to Production" --limit=1 --json databaseId,status,conclusion,updatedAt
```
Poll every 15 seconds (up to 5 minutes) until `status` is `completed`.

### 4. Report final status
Show a summary table:

| Environment | Action | Status | Run |
|-------------|--------|--------|-----|
| Staging     | Source (no change) | ✅ success | #XXXXXXXXX |
| UAT         | Promoted from staging | ✅ success | #XXXXXXXXX |
| Production  | Promoted from staging | ✅ success | #XXXXXXXXX |

URLs:
- Staging:    https://staging.taskai.cc
- UAT:        https://uat.taskai.cc
- Production: https://taskai.cc

Monitor all runs: https://github.com/anchoo2kewl/taskai/actions
