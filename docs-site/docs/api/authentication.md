---
sidebar_position: 1
---

# Authentication

TaskAI supports two authentication methods: **JWT Bearer Tokens** and **API Keys**.

## JWT Bearer Tokens

Obtained by signing up or logging in. Include in all authenticated requests:

```
Authorization: Bearer <token>
```

### Sign Up

```bash
curl -X POST https://taskai.cc/api/auth/signup \
  -H "Content-Type: application/json" \
  -d '{
    "email": "user@example.com",
    "password": "SecurePass123!"
  }'
```

**Response (201):**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "user": {
    "id": 1,
    "email": "user@example.com",
    "created_at": "2025-10-18T00:00:00Z"
  }
}
```

**Password Requirements:**
- Minimum 8 characters
- At least one letter and one digit

### Log In

```bash
curl -X POST https://taskai.cc/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "user@example.com",
    "password": "SecurePass123!"
  }'
```

**Response (200):** Same format as signup.

If 2FA is enabled, you'll receive a `2fa_required` response and must include the TOTP code:

```bash
curl -X POST https://taskai.cc/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "user@example.com",
    "password": "SecurePass123!",
    "totp_code": "123456"
  }'
```

### Get Current User

```bash
curl https://taskai.cc/api/me \
  -H "Authorization: Bearer YOUR_TOKEN"
```

## API Keys

For programmatic access. Create keys in **Settings** → **API Keys** or via the API (see [API Keys guide](/guides/api-keys)).

```
Authorization: ApiKey <key>
```

API keys have the same permissions as the user who created them.

## OAuth Login

TaskAI supports social login via Google and GitHub. See [OAuth Setup](/guides/oauth-setup) for details.

## Rate Limiting

| Endpoint Type | Limit |
|--------------|-------|
| Auth endpoints (`/api/auth/*`) | 20 requests/minute |
| Protected endpoints | 100 requests/minute |

Rate limit headers are included in every response:
```
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1634567890
```
