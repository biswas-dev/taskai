# Session Context

## User Prompts

### Prompt 1

Implement the following plan:

# Plan: Rename SprintSpark → TaskAI + Pipeline Alignment

## Context

The project identity has shifted from "SprintSpark" to "TaskAI" (taskai.cc is already the production domain). This plan renames all code, config, infra references, and aligns the CI/CD pipeline with the folioworth pattern (webhook-based, CI-gated staging, manual production promotion).

### Key Decisions
- Staging domain: `sprintspark.biswas.me` → `staging.taskai.cc`
- MCP staging domain: `mcp...

