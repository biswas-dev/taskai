# TaskAI ⚡

[![CI](https://github.com/anchoo2kewl/taskai/actions/workflows/ci.yml/badge.svg)](https://github.com/anchoo2kewl/taskai/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/anchoo2kewl/taskai)](https://goreportcard.com/report/github.com/anchoo2kewl/taskai)
[![License: Proprietary](https://img.shields.io/badge/License-Proprietary-red.svg)](LICENSE)

> A lightweight, production-grade project management system built with Go, SQLite, React, and TypeScript.

**Philosophy:** Small, perfect commits > large, broken features

---

## ⚡ Quick Start (< 3 commands)

```bash
git clone https://github.com/anchoo2kewl/taskai.git
cd taskai
docker-compose up --build
```

**That's it!** Visit [http://localhost](http://localhost) 🎉

---

## 📋 Table of Contents

- [Features](#-features)
- [Architecture](#-architecture)
- [Getting Started](#-getting-started)
- [API Documentation](#-api-documentation)
- [Testing](#-testing)
- [Deployment](#-deployment)
- [Contributing](#-contributing)
- [Troubleshooting](#-troubleshooting)
- [License](#-license)

---

## ✨ Features

### Core Functionality
- 📊 **Project Management** - Create, organize, and track multiple projects
- ✅ **Task Tracking** - Search, filter, and manage tasks with rich metadata
- 🔐 **Authentication** - Secure JWT-based auth with bcrypt password hashing
- 🔍 **Full-Text Search** - Find tasks and projects instantly
- 📱 **Responsive UI** - Works on desktop and mobile
- 🧠 **Memories** - One store of short lessons per person, for every AI and app you use. Write and search over REST or MCP. See the [memory protocol and quick start](docs/memory-protocol.md).

### Technical Features
- 🚀 **RESTful API** - Complete OpenAPI 3.1 specification
- 🎨 **Modern Stack** - React 18 + TypeScript + Tailwind CSS
- 💾 **SQLite Database** - Zero-config, portable, reliable
- 🐳 **Docker Ready** - Multi-stage builds for production
- 🔄 **Auto-Deploy** - GitHub webhook integration
- ✅ **Type-Safe** - End-to-end TypeScript with generated API types
- 🧪 **Well-Tested** - Unit tests + E2E tests with Playwright
- 📊 **CI/CD** - GitHub Actions with security scanning

---

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────────────┐
│                     Client Browser                      │
│                    (React + TypeScript)                 │
└────────────────────┬────────────────────────────────────┘
                     │ HTTPS
                     ▼
┌─────────────────────────────────────────────────────────┐
│                   Nginx (Port 80/443)                   │
│              (Gzip, Security Headers, SPA)              │
└────────────────────┬────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────┐
│                  Go API (Port 8080)                     │
│         (JWT Auth, OpenAPI, Rate Limiting)              │
└────────────────────┬────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────┐
│                   SQLite Database                       │
│            (Migrations, Foreign Keys, WAL)              │
└─────────────────────────────────────────────────────────┘
```

### Component Breakdown

| Component | Technology | Purpose |
|-----------|-----------|---------|
| **Frontend** | React 18, TypeScript, Vite, Tailwind CSS | SPA with type-safe API client |
| **Backend** | Go 1.24, Chi router, JWT | RESTful API with authentication |
| **Database** | SQLite 3 (WAL mode) | Embedded database with migrations |
| **Testing** | Playwright, Go testing | E2E and unit tests |
| **CI/CD** | GitHub Actions, Docker | Automated testing and deployment |
| **Deployment** | Docker Compose, Nginx | Production containers |

---

## 🚀 Getting Started

### Prerequisites

**Option 1: Docker (Recommended)**
- Docker Desktop or Docker Engine 20+
- Docker Compose v2+

**Option 2: Local Development**
- Go 1.24+
- Node.js 20+
- npm 10+

### Installation & Setup

#### **Option 1: Docker (Production Build)**

```bash
# Clone the repository
git clone https://github.com/anchoo2kewl/taskai.git
cd taskai

# Start all services
docker-compose up --build -d

# Check status
docker-compose ps
```

Access the application:
- **Frontend:** http://localhost
- **API:** http://localhost:8080
- **API Docs:** http://localhost:8080/api/openapi

#### **Option 2: Local Development (Hot Reload)**

```bash
# Clone the repository
git clone https://github.com/anchoo2kewl/taskai.git
cd taskai

# Start everything with hot-reload
make dev
```

This starts:
- API with Air hot-reload on http://localhost:8080
- Web with Vite HMR on http://localhost:5173

#### **Option 3: Manual Setup**

**Backend:**
```bash
cd api

# Copy environment file
cp .env.example .env

# Install dependencies
go mod download

# Run migrations
make migrate

# Start API server
make run
```

**Frontend:**
```bash
cd web

# Copy environment file
cp .env.example .env

# Install dependencies
npm install

# Generate TypeScript types from OpenAPI
npm run generate:types

# Start dev server
npm run dev
```

### First Steps

1. **Create an account** - Navigate to http://localhost:5173/signup
2. **Login** - Use your credentials
3. **Create a project** - Click "New Project"
4. **Add tasks** - Start tracking your work!

---

## 📚 API Documentation

### Interactive Documentation

The OpenAPI specification is available at:
- **JSON:** http://localhost:8080/api/openapi
- **Interactive UI:** Use Swagger UI or Postman

### API Examples

See [docs/API_EXAMPLES.md](docs/API_EXAMPLES.md) for complete examples.

**Quick Reference:**

```bash
# Register new user
curl -X POST http://localhost:8080/api/auth/signup \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"SecurePass123!"}'

# Login
curl -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"SecurePass123!"}'
# Returns: {"token": "eyJhbGc...", "user": {...}}

# Create project (authenticated)
curl -X POST http://localhost:8080/api/projects \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"My Project","description":"Project description"}'

# List projects
curl http://localhost:8080/api/projects \
  -H "Authorization: Bearer YOUR_TOKEN"

# Create task
curl -X POST http://localhost:8080/api/projects/1/tasks \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"title":"Implement feature","status":"todo","priority":"high"}'

# Search tasks
curl "http://localhost:8080/api/projects/1/tasks?query=feature" \
  -H "Authorization: Bearer YOUR_TOKEN"
```

### TypeScript API Client

The frontend uses auto-generated types from the OpenAPI spec:

```typescript
import { api } from './lib/api'

// All API calls are type-safe
const projects = await api.getProjects()
const project = await api.createProject({
  name: 'New Project',
  description: 'Description'
})
```

---

## 🧪 Testing

### Running Tests

```bash
# Backend tests (unit + integration)
cd api
make test                    # Run all tests
make test-coverage          # With coverage report

# Frontend tests
cd web
npm run test                # Unit tests
npm run type-check          # TypeScript validation

# E2E tests (requires running app)
cd web
npx playwright test         # Headless
npx playwright test --headed # With browser
npx playwright test --ui    # Interactive mode
```

### Test Structure

**Backend Tests:**
- `internal/api/*_test.go` - HTTP handler tests
- `internal/auth/*_test.go` - Auth logic tests
- `internal/e2e/e2e_test.go` - Full API flow tests

**Frontend Tests:**
- `tests/auth-and-crud.spec.ts` - Complete user journey
- `tests/helpers.ts` - Reusable test utilities

### Coverage Goals

- **Backend:** 80%+ for critical paths
- **Frontend:** Complete user journeys in E2E tests
- **API:** All endpoints tested in E2E suite

---

## 🚢 Deployment

### Production Deployment (Docker)

```bash
# Build production images
docker-compose up --build -d

# View logs
docker-compose logs -f

# Stop services
docker-compose down

# Stop and remove data
docker-compose down -v
```

### Environment Variables

**API (`api/.env`):**
```env
DB_PATH=/data/taskai.db
JWT_SECRET=your-secure-secret-here
PORT=8080
ENV=production
CORS_ALLOWED_ORIGINS=https://yourdomain.com
LOG_LEVEL=info
```

**Web (`web/.env`):**
```env
VITE_API_URL=http://localhost:8080
```

### Auto-Deployment with Webhooks

TaskAI includes a webhook system for automatic deployment:

1. **Configure webhook on server:**
   ```bash
   cd webhook
   cp config.sample.yaml config.yaml
   # Edit config.yaml with your settings
   python3 webhook_server.py
   ```

2. **Add GitHub webhook:**
   - URL: `https://webhook.yourdomain.com/webhook/taskai`
   - Secret: (from your config.yaml)
   - Events: Push events

See [webhook/README.md](webhook/README.md) for detailed setup.

### Production Checklist

- [ ] Change `JWT_SECRET` to a strong random value
- [ ] Update `CORS_ALLOWED_ORIGINS` to your domain
- [ ] Set up HTTPS with Let's Encrypt
- [ ] Configure database backups
- [ ] Set up log rotation
- [ ] Enable firewall rules
- [ ] Configure webhook for auto-deployment
- [ ] Set up monitoring/alerts

---

## 🤝 Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for:

- Code style guidelines (Go & TypeScript)
- Commit message conventions
- Testing requirements
- Pull request process

**Quick Start for Contributors:**

1. Fork the repository
2. Create a feature branch: `git checkout -b feat/amazing-feature`
3. Make your changes
4. Run tests: `make test` (api) and `npm test` (web)
5. Commit: `git commit -m "feat: Add amazing feature"`
6. Push: `git push origin feat/amazing-feature`
7. Open a Pull Request

See [CLAUDE.md](CLAUDE.md) for AI-assisted development guidelines.

---

## 🔧 Troubleshooting

See [docs/TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md) for detailed troubleshooting.

**Common Issues:**

### Database locked error
```bash
# Stop all running instances
docker-compose down
# Remove old database
rm api/data/taskai.db*
# Restart
docker-compose up
```

### Port already in use
```bash
# Find process using port 8080
lsof -ti:8080 | xargs kill -9
# Or use different port in .env
```

### CORS errors
```bash
# Check CORS_ALLOWED_ORIGINS in api/.env
# Add your frontend URL (e.g., http://localhost:5173)
```

### TypeScript errors after API changes
```bash
cd web
npm run generate:types  # Regenerate from OpenAPI spec
npm run type-check      # Verify
```

---

## 📖 Additional Documentation

- [API Examples](docs/API_EXAMPLES.md) - Complete API usage guide
- [Troubleshooting](docs/TROUBLESHOOTING.md) - Common issues and solutions
- [Contributing](CONTRIBUTING.md) - How to contribute
- [Development Guide](CLAUDE.md) - AI-assisted development
- [Webhook Setup](webhook/README.md) - Auto-deployment configuration

---

## 📝 License

Proprietary. Copyright (c) 2026 Anshuman Biswas. All rights reserved. See [LICENSE](LICENSE). TaskAI is not open source.

[Brain Hub (llm-brain)](https://github.com/anchoo2kewl/llm-brain), the companion agent hub, is a separate open-source project under the [MIT license](https://github.com/anchoo2kewl/llm-brain/blob/main/LICENSE).

---

## 🙏 Acknowledgments

Built with:
- [Go](https://golang.org/) - Backend language
- [Chi](https://github.com/go-chi/chi) - HTTP router
- [React](https://react.dev/) - UI framework
- [Vite](https://vitejs.dev/) - Build tool
- [Tailwind CSS](https://tailwindcss.com/) - Styling
- [Playwright](https://playwright.dev/) - E2E testing
- [Docker](https://www.docker.com/) - Containerization

---

**Made with ⚡ by the TaskAI team**

🤖 Generated with [Claude Code](https://claude.com/claude-code)
