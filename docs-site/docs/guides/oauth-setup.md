---
sidebar_position: 3
---

# OAuth Login

TaskAI supports social login via Google and GitHub, so your team can sign in without creating separate passwords.

## Supported Providers

- **Google** — Sign in with your Google account
- **GitHub** — Sign in with your GitHub account

## How It Works

1. Click **Sign in with Google** or **Sign in with GitHub** on the login page
2. Authorize TaskAI with the OAuth provider
3. You're logged in — a TaskAI account is created automatically if it's your first time

## Invite-Gated Access

OAuth login is invite-gated. Users must be invited to a team or project before they can sign in with OAuth. This prevents unauthorized signups while still allowing social login convenience.

## Security

- All OAuth flows use state parameters for CSRF protection
- OAuth tokens are stored server-side and never exposed to the client
- Separate OAuth apps are used for login vs. GitHub repo sync
