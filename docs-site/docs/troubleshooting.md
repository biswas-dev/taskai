---
sidebar_position: 100
---

# Troubleshooting

Common issues and solutions when using TaskAI.

## Authentication Issues

### Cannot Sign Up

**Password requirements:**
- Minimum 8 characters
- At least one letter and one digit

### Logged Out Unexpectedly

JWT tokens expire after 24 hours. Log in again to get a fresh token. If using OAuth (Google/GitHub), click the sign-in button to re-authenticate.

### 2FA Code Not Working

- Ensure your authenticator app's clock is synced
- TOTP codes are time-based and valid for 30 seconds
- If locked out, contact support at support@taskai.cc

---

## API Issues

### 401 Unauthorized

Your token may be expired or malformed. Log in again to get a fresh token:

```bash
curl -X POST https://taskai.cc/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email": "you@example.com", "password": "your-password"}'
```

### 403 Forbidden

You don't have permission to access this resource. Check that:
- You're a member of the project
- You have the required role (`member`, `admin`, or `owner`)

### 429 Too Many Requests

You've hit the rate limit. Wait and retry:
- Auth endpoints: 20 requests/minute
- Other endpoints: 100 requests/minute

Check `X-RateLimit-Remaining` and `X-RateLimit-Reset` headers.

### CORS Errors

If you're calling the API from a browser on a different domain, CORS may block the request. The TaskAI API at `taskai.cc` allows requests from the TaskAI web app. For custom integrations, use server-side requests or API keys.

---

## MCP Issues

### Agent Can't Connect

1. Verify your API key is valid: try `curl -H "Authorization: ApiKey YOUR_KEY" https://taskai.cc/api/me`
2. Check the MCP server URL is `https://mcp.taskai.cc/sse`
3. Ensure `mcp-remote` is installed: `npx -y mcp-remote --help`

### Tools Return Empty Results

- Check that your API key has access to the project you're querying
- Use `list_projects` first to verify accessible projects
- Try `verbose: true` to get full details

---

## GitHub Sync Issues

### Issues Not Syncing

- Verify GitHub is connected in **Settings** → **Integrations**
- Check that sync is enabled for the project in **Project Settings** → **GitHub**
- Try a manual sync via the project settings page

### Comments Not Appearing

Comments sync may take a few moments. If comments still don't appear:
- Verify the GitHub issue is linked to a TaskAI task
- Check that the GitHub OAuth token hasn't expired (reconnect if needed)

---

## General Issues

### Page Not Loading

1. Clear your browser cache and cookies
2. Try a different browser or incognito mode
3. Check [status.taskai.cc](https://status.taskai.cc) for service status

### Search Not Finding Results

Full-text search indexes are updated in real-time. If a recently created task isn't appearing:
- Ensure you're searching in the correct project
- Try more specific search terms
- Check that the task title or description contains the search query

---

## Getting Help

- Email: support@taskai.cc
- API status: Check `https://taskai.cc/api/health`
