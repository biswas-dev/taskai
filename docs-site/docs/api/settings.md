---
sidebar_position: 16
---

# User Settings

Manage passwords, two-factor authentication, and account integrations.

## Change Password

```bash
curl -X POST https://taskai.cc/api/settings/password \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "current_password": "OldPass123!",
    "new_password": "NewPass456!"
  }'
```

## Two-Factor Authentication (2FA)

TaskAI supports TOTP-based 2FA using apps like Google Authenticator or Authy.

### Enable 2FA

**Step 1:** Generate a TOTP secret:

```bash
curl -X POST https://taskai.cc/api/settings/2fa/setup \
  -H "Authorization: Bearer YOUR_TOKEN"
```

**Response:** Returns a TOTP secret and QR code URL. Scan the QR code with your authenticator app.

**Step 2:** Verify and activate:

```bash
curl -X POST https://taskai.cc/api/settings/2fa/verify \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"code": "123456"}'
```

### Disable 2FA

```bash
curl -X DELETE https://taskai.cc/api/settings/2fa \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"code": "123456"}'
```

Requires a valid TOTP code to disable.

## Cloudinary Settings

Configure image upload settings for the project:

```bash
curl -X PATCH https://taskai.cc/api/settings/cloudinary \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "cloud_name": "my-cloud",
    "upload_preset": "taskai-uploads"
  }'
```

## Figma Integration

Connect Figma for design embedding:

```bash
curl -X PATCH https://taskai.cc/api/settings/figma \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "figma_token": "your-figma-personal-access-token"
  }'
```
