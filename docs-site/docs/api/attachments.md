---
sidebar_position: 13
---

# Attachments

Upload and manage file attachments on tasks and wiki pages. Files are stored via Cloudinary.

## Upload Attachment

```bash
curl -X POST https://taskai.cc/api/tasks/1/attachments \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -F "file=@screenshot.png"
```

**Response (201):**
```json
{
  "id": 1,
  "filename": "screenshot.png",
  "url": "https://res.cloudinary.com/...",
  "content_type": "image/png",
  "size": 245760,
  "created_at": "2025-10-18T00:00:00Z"
}
```

## List Attachments

```bash
curl https://taskai.cc/api/tasks/1/attachments \
  -H "Authorization: Bearer YOUR_TOKEN"
```

## Delete Attachment

```bash
curl -X DELETE https://taskai.cc/api/attachments/1 \
  -H "Authorization: Bearer YOUR_TOKEN"
```

## Cloudinary Setup

To enable file uploads, configure Cloudinary credentials in your environment:

```bash
CLOUDINARY_CLOUD_NAME=your-cloud-name
CLOUDINARY_API_KEY=your-api-key
CLOUDINARY_API_SECRET=your-api-secret
```

Supported file types include images, PDFs, and common document formats. Maximum file size is configurable.
