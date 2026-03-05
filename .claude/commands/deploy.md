Deploy TaskAI to all environments: staging → UAT → production.

## Usage
```
/deploy "feat: your commit message here"
```

If no message is provided as `$ARGUMENTS`, use a generic message based on recent changes.

## Rules
- UAT is ALWAYS promoted from staging (main), never developed on directly
- Production is ALWAYS promoted from main after staging is verified
- Never manually SSH to servers — always use GitHub workflows

## Steps

### 1. Deploy to Staging
Run `./script/server deploy "$ARGUMENTS"` (commits uncommitted changes, pushes to main, triggers GitHub Actions staging deploy).

If there are no uncommitted changes and `$ARGUMENTS` is empty, just push: `git push origin main`.

### 2. Promote to UAT
```bash
./script/server promote staging uat
```

### 3. Promote to Production
```bash
./script/server promote staging prod
# or, if UAT was verified and you want to promote from UAT:
./script/server promote uat prod
```

You can chain steps 2 and 3:
```bash
./script/server promote staging uat && ./script/server promote staging prod
```

### 4. Report Status
Show a summary table:
| Environment | Action | URL |
|---|---|---|
| Staging | Deployed via push to main | https://staging.taskai.cc |
| UAT | Force-promoted from main | https://uat.taskai.cc |
| Production | GitHub Actions triggered | https://taskai.cc |

Monitor: https://github.com/anchoo2kewl/taskai/actions
