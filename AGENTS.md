# Project

Self-hosted music server via Docker Compose.
- **Navidrome** (deluan/navidrome) — Subsonic streaming server at `${NAVIDROME_DOMAIN}`
- **MeTube** (ghcr.io/alexta69/metube) — yt-dlp web UI, доступен только через auth-panel (iframe)
- **Auth Panel** (Go) — единая точка входа: регистрация по инвайтам + загрузка музыки + прокси к MeTube at `${AUTH_PANEL_DOMAIN}`
- **Traefik** (v3) — reverse proxy + Let's Encrypt SSL (external, in `web` network)

# Directories

- `docker-compose.yml` — production config (Traefik, без портов)
- `docker-compose.local.yml` — локальный запуск (прямые порты, bind mount)
- `./data/` — Navidrome DB and cache
- `./music/` — shared library: `ro` для Navidrome в production, `rw` для MeTube и auth-panel
- `./auth-db-data/` — PostgreSQL volume для auth-panel
- `./auth-panel/` — Go source (cmd/auth-panel/main.go, internal/, web/)

# Docker Compose Rules

- Use `docker compose` (v2 syntax), not `docker-compose`
- All containers: `restart: unless-stopped`
- Escape `$` in env vars as `$$` (e.g. Traefik labels)
- Preserve existing Traefik labels when editing
- File permissions: Navidrome `user: 1000:1000`; MeTube `UID=1000`, `GID=1000`
- No default passwords — все значения из `.env`

# Traefik

- Entrypoint: `websecure`; `tls=true` всегда
- MeTube НЕ имеет Traefik labels — доступен только через auth-panel прокси
- Объясняй изменения Traefik labels — ошибка ломает роутинг

# Auth Panel (Go)

- **Единая точка входа** — `/login`, после входа сессия (http-only cookie `session`, 24h)
- `/` → `/login` (если нет сессии) или `/upload` (если есть)
- Регистрация только по инвайт-ссылкам: `/register?invite=<code>`
- Пароли проверяются через Navidrome Auth API (`POST /auth/login`) — единый источник
- Админ-панель: `/admin` — создание инвайтов (доступ только для `is_admin` в local DB)
- Создание пользователей в Navidrome через Navidrome Admin API (`POST /api/user` с токеном из `/auth/login`)
- Загрузка музыки: `/upload` — авторизация через сессию
- MeTube прокси: `/metube/` — через iframe с тулбаром, авторизация через сессию
- Upload поддерживает отдельные аудиофайлы и zip-архивы (сохраняет структуру папок)
- PostgreSQL только на internal network (не暴露 в Traefik)
- Build: multi-stage Dockerfile (golang:1.25-alpine → alpine + unzip)
- Dev: `go run ./cmd/auth-panel` (нужны env vars)
- Зависимости: `github.com/gorilla/websocket`, `github.com/lib/pq`

# Local DB Schema (PostgreSQL)

```sql
CREATE TABLE invites (id UUID, code TEXT UNIQUE, used BOOLEAN, created_at TIMESTAMP);
CREATE TABLE users (id UUID, username TEXT UNIQUE, name TEXT, is_admin BOOLEAN, created_at TIMESTAMP);
```

# Workflow

- Ответы на русском, кратко
- Пользователь в группе `docker` — без `sudo`
- Создавать директории через `bash` перед `docker compose up`
- Required env vars: `NAVIDROME_ADMIN_USER`, `NAVIDROME_ADMIN_PASSWORD`, `AUTH_DB_PASSWORD`
- Перед первым запуском: `cp .env.example .env` и заполнить своими значениями
- При первом запуске: создать админа в Navidrome через `localhost:4533` (один раз)

# Decision Rules

- ALWAYS спрашивать пользователя перед архитектурными решениями (новые сервисы, схемы авторизации, прокси, API)
- При наличии нескольких вариантов реализации — предложить выбор
- Не предполагать предпочтения пользователя по UX, безопасности, фичам — спрашивать
- Действовать без вопросов только для тривиальных фиксов (опечатки, очевидные баги, форматирование)
- **Перед `git push` — обязательно согласовать с пользователем**

# Git Workflow

- После завершения каждой логической задачи делать `git commit` автоматически (без напоминания пользователя)
- **Один коммит = одна логическая задача** (не смешивать разные фичи/багфиксы в один коммит)
- Сообщение коммита должно отражать суть изменений (feat/fix/refactor/docs)
