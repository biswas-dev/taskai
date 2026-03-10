---
sidebar_position: 100
---

# Troubleshooting

Common issues and solutions for TaskAI.

## Installation Issues

### Go Version Mismatch

**Problem:** `go.mod requires go >= 1.24.0`

```bash
# Check version
go version

# macOS
brew upgrade go

# Linux
wget https://go.dev/dl/go1.24.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.24.linux-amd64.tar.gz
```

### Node Version Mismatch

```bash
# Install Node 20+ using nvm
nvm install 20
nvm use 20
```

---

## Database Issues

### Migration Failed

```bash
# Reset database
docker compose exec postgres psql -U taskai -c "DROP DATABASE taskai;"
docker compose exec postgres psql -U taskai -c "CREATE DATABASE taskai;"
docker compose restart api
```

### Cannot Connect to Database

```bash
# Check PostgreSQL is running
docker compose ps postgres

# Check connection
docker compose exec postgres pg_isready -U taskai
```

---

## API Issues

### Port Already in Use

```bash
# Find process using port 8080
lsof -ti:8080 | xargs kill -9

# Or change port in .env
TASKAI_API_PORT=8081
```

### CORS Errors

Check `CORS_ALLOWED_ORIGINS` in your `.env` file includes your frontend URL:

```bash
CORS_ALLOWED_ORIGINS=http://localhost:5173,https://yourdomain.com
```

### JWT Token Validation Failed

Causes:
1. `JWT_SECRET` changed after token was issued
2. Token expired (default 24h)
3. Token malformed

Solution: Log out and log back in. Ensure `JWT_SECRET` is consistent across restarts.

---

## Frontend Issues

### TypeScript Errors After API Changes

```bash
cd web
npm run generate:types  # Regenerate from OpenAPI spec
npm run type-check
```

### Blank Page / White Screen

1. Check browser console for errors
2. Verify API is accessible: `curl http://localhost:8080/api/health`
3. Clear localStorage: `localStorage.clear()` in browser console
4. Restart: `rm -rf dist/ && npm run dev`

---

## Docker Issues

### Docker Build Fails

```bash
# Clear cache and rebuild
docker system prune -a
docker compose build --no-cache
```

### Container Exits Immediately

```bash
# Check logs
docker compose logs api
docker compose logs web

# Run in foreground
docker compose up
```

### Volume Permission Issues

```bash
docker compose down -v  # Remove volumes
docker compose up --build  # Start fresh
```

---

## Authentication Issues

### Cannot Sign Up

Password requirements:
- Minimum 8 characters
- At least one letter and one digit

### Logged Out After Refresh

Check that `JWT_SECRET` hasn't changed. Tokens are stored in `localStorage` and validated against the current secret.

---

## Performance Issues

### Slow API Response

```bash
# Check database indexes
docker compose exec postgres psql -U taskai -c "\di"

# Analyze queries
docker compose exec postgres psql -U taskai -c "ANALYZE;"
```

### Slow Docker Builds

```bash
# Build only the changed service
docker compose build api
docker compose up -d api

# Clean unused images
docker image prune -a
```

---

## Testing Issues

### E2E Tests Fail

```bash
cd web
npx playwright install --with-deps
npx playwright test --headed  # See what's happening
```

### Go Tests Fail

```bash
cd api
go test -v -run TestName ./internal/api  # Run specific test
go test -race ./...                       # Check race conditions
go clean -testcache                       # Clear cache
```

---

## Getting Help

1. Check logs: `docker compose logs -f`
2. Verify versions: `go version`, `node --version`, `docker --version`
3. Try fresh install: `docker compose down -v && docker compose up --build`
4. Open an issue: [github.com/anchoo2kewl/taskai/issues](https://github.com/anchoo2kewl/taskai/issues)
