# Project

Self-hosted music server via Docker Compose.
- **Navidrome** (deluan/navidrome) — Subsonic streaming server at `${NAVIDROME_DOMAIN}`
- **MeTube** (ghcr.io/alexta69/metube) — yt-dlp web UI, доступен только через auth-panel
- **Auth Panel** (Go) — регистрация по инвайтам + загрузка музыки + прокси к MeTube at `${AUTH_PANEL_DOMAIN}`
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

- Регистрация только по инвайт-ссылкам: `/register?invite=<code>`
- Админ-панель: `/admin` — создание инвайтов, защищена `ADMIN_PASSWORD_HASH` (bcrypt)
- Создание пользователей в Navidrome через Navidrome Admin API (`POST /api/user` с токеном из `/auth/login`)
- Загрузка музыки: `/upload` — авторизация через учётные данные Navidrome (проверка через `/rest/ping`)
- Сессии: после входа сервер ставит http-only cookie `session` (24h), запросы без пароля
- MeTube прокси: `/metube/` — авторизация через cookie, WebSocket через gorilla/websocket
- Upload поддерживает отдельные аудиофайлы и zip-архивы (сохраняет структуру папок)
- PostgreSQL только на internal network (не暴露 в Traefik)
- Build: multi-stage Dockerfile (golang:1.22-alpine → alpine + unzip)
- Dev: `go run ./cmd/auth-panel` (нужны env vars)
- Зависимости: `github.com/gorilla/websocket`, `github.com/lib/pq`, `golang.org/x/crypto`
- Утилита генерации хеша: `go run ./cmd/auth-panel/cmd/genhash <password>`

# Workflow

- Ответы на русском, кратко
- Пользователь в группе `docker` — без `sudo`
- Создавать директории через `bash` перед `docker compose up`
- Required env vars: `NAVIDROME_ADMIN_USER`, `NAVIDROME_ADMIN_PASSWORD`, `AUTH_ADMIN_PASSWORD_HASH`, `AUTH_DB_PASSWORD`
- Перед первым запуском: `cp .env.example .env` и заполнить своими значениями

# Decision Rules

- ALWAYS спрашивать пользователя перед архитектурными решениями (новые сервисы, схемы авторизации, прокси, API)
- При наличии нескольких вариантов реализации — предложить выбор
- Не предполагать предпочтения пользователя по UX, безопасности, фичам — спрашивать
- Действовать без вопросов только для тривиальных фиксов (опечатки, очевидные баги, форматирование)
