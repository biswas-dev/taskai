---
sidebar_position: 17
---

# Error Handling

All API errors return a consistent JSON format with an HTTP status code.

## Error Response Format

```json
{
  "error": "Human-readable error message",
  "code": "machine_readable_code"
}
```

## Status Codes

| HTTP Status | Code | Description |
|-------------|------|-------------|
| 400 | `invalid_request` | Malformed request body or missing required fields |
| 401 | `unauthorized` | Missing or invalid authentication token |
| 403 | `forbidden` | User doesn't have permission for this resource |
| 404 | `not_found` | Resource doesn't exist |
| 409 | `conflict` | Resource already exists (e.g., duplicate email) |
| 422 | `validation_error` | Input validation failed |
| 429 | `rate_limit_exceeded` | Too many requests |
| 500 | `internal_error` | Server error |

## Example Error Responses

**Validation Error (400):**
```json
{
  "error": "Invalid email format",
  "code": "validation_error"
}
```

**Unauthorized (401):**
```json
{
  "error": "Authentication required",
  "code": "unauthorized"
}
```

**Not Found (404):**
```json
{
  "error": "Project not found",
  "code": "not_found"
}
```

**Conflict (409):**
```json
{
  "error": "Email already registered",
  "code": "conflict"
}
```

**Rate Limited (429):**
```json
{
  "error": "Rate limit exceeded. Please try again later.",
  "code": "rate_limit_exceeded"
}
```

## Best Practices

1. **Check the `code` field** for programmatic error handling, not the `error` message
2. **Implement retry logic** with exponential backoff for 429 and 500 errors
3. **Log the full response** when debugging unexpected errors
4. **Respect rate limit headers** (`X-RateLimit-Remaining`) to avoid 429s
