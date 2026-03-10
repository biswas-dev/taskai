---
sidebar_position: 15
---

# Drawings

TaskAI includes a built-in canvas editor for creating diagrams, wireframes, and architecture docs. Drawings can be embedded in wiki pages.

## List Project Drawings

```bash
curl https://taskai.cc/api/projects/1/drawings \
  -H "Authorization: Bearer YOUR_TOKEN"
```

## Create Drawing

```bash
curl -X POST https://taskai.cc/api/projects/1/drawings \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "System Architecture"
  }'
```

**Response (201):**
```json
{
  "id": "abc123",
  "title": "System Architecture",
  "shortcode": "[draw:abc123:edit:m]"
}
```

## Get Drawing

```bash
curl https://taskai.cc/api/drawings/abc123 \
  -H "Authorization: Bearer YOUR_TOKEN"
```

Returns the drawing metadata and scene data.

## Save Drawing Scene

Update the drawing canvas content programmatically:

```bash
curl -X PUT https://taskai.cc/api/drawings/abc123 \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "System Architecture v2",
    "scene": {
      "version": 1,
      "elements": [
        {
          "type": "rect",
          "x": 100, "y": 100,
          "w": 200, "h": 80,
          "text": "API Server",
          "fillColor": "#dbeafe",
          "strokeColor": "#3b82f6"
        },
        {
          "type": "arrow",
          "x": 200, "y": 180,
          "x2": 200, "y2": 280
        },
        {
          "type": "rect",
          "x": 100, "y": 280,
          "w": 200, "h": 80,
          "text": "PostgreSQL",
          "fillColor": "#fef3c7",
          "strokeColor": "#f59e0b"
        }
      ]
    }
  }'
```

### Scene Element Types

| Type | Properties | Description |
|------|-----------|-------------|
| `rect` | `x, y, w, h, text` | Rectangle with optional text |
| `ellipse` | `x, y, w, h, text` | Ellipse with optional text |
| `arrow` | `x, y, x2, y2` | Arrow from (x,y) to (x2,y2) |
| `line` | `x, y, x2, y2` | Line from (x,y) to (x2,y2) |
| `text` | `x, y, w, h, text, fontSize` | Text element |
| `pencil` | `pts: [{x, y}]` | Freehand drawing |

### Common Style Properties

| Property | Type | Description |
|----------|------|-------------|
| `strokeColor` | string | Border/line color (hex) |
| `fillColor` | string | Fill color (hex) |
| `opacity` | number | Opacity 0–100 |
| `strokeWidth` | number | Line width 1–4 |
| `angle` | number | Rotation in radians |

## Embedding in Wiki

Use the shortcode to embed a drawing in any wiki page:

```markdown
Here is our architecture diagram:

[draw:abc123:edit:m]
```

The drawing renders inline and can be edited directly in the wiki page.
