---
sidebar_position: 3
---

# OAuth Setup

Enable social login via Google and GitHub.

## Google OAuth

### 1. Create OAuth Credentials

1. Go to [Google Cloud Console](https://console.cloud.google.com/apis/credentials)
2. Create a new OAuth 2.0 Client ID
3. Set Authorized redirect URIs:
   - `https://yourdomain.com/api/auth/google/callback`
   - `http://localhost:8080/api/auth/google/callback` (for dev)

### 2. Configure Environment

```bash
GOOGLE_CLIENT_ID=your-google-client-id.apps.googleusercontent.com
GOOGLE_CLIENT_SECRET=your-google-client-secret
OAUTH_STATE_SECRET=$(openssl rand -hex 32)
OAUTH_SUCCESS_URL=https://yourdomain.com/oauth/callback
OAUTH_ERROR_URL=https://yourdomain.com/login?error=oauth
```

## GitHub OAuth (Login)

### 1. Create OAuth App

1. Go to [GitHub Developer Settings](https://github.com/settings/developers)
2. Create a new OAuth App
3. Set Authorization callback URL:
   - `https://yourdomain.com/api/auth/github/login/callback`

### 2. Configure Environment

```bash
LOGIN_GITHUB_CLIENT_ID=your-github-client-id
LOGIN_GITHUB_CLIENT_SECRET=your-github-client-secret
```

:::note
This is separate from the GitHub Integration OAuth app (used for repo sync). Login OAuth and repo sync OAuth use different client IDs.
:::

## How It Works

1. User clicks "Sign in with Google/GitHub" on the login page
2. Browser redirects to the OAuth provider
3. User authorizes the application
4. Provider redirects back to `/api/auth/{provider}/callback`
5. API creates or finds the user account and issues a JWT
6. Browser redirects to `OAUTH_SUCCESS_URL` with the token

## Invite-Gated Access

OAuth login is invite-gated by default. Users must be invited (via email) before they can sign in with OAuth. This prevents unauthorized signups while still allowing social login convenience.

## Security Notes

- `OAUTH_STATE_SECRET` is used for CSRF-proof state JWTs — different from `JWT_SECRET`
- OAuth tokens are stored server-side and never exposed to the client
- All OAuth flows use PKCE or state parameters for security
