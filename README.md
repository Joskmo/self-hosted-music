# Music Server

Self-hosted музыкальный сервер с регистрацией по инвайтам, загрузкой музыки и скачиванием с YouTube.

## Компоненты

- **[Navidrome](https://www.navidrome.org/)** — музыкальный стриминговый сервер (Subsonic API)
- **[MeTube](https://github.com/alexta69/metube)** — скачивание видео/аудио с YouTube и других сайтов
- **Auth Panel** (Go) — регистрация по инвайтам, загрузка музыки, прокси к MeTube
- **[Traefik](https://traefik.io/)** — reverse proxy + SSL (production)

## Возможности

- Регистрация пользователей только по пригласительным ссылкам
- Загрузка музыки через веб-интерфейс (файлы и zip-архивы)
- Скачивание с YouTube через защищённый прокси
- Все сервисы защищены авторизацией через Navidrome

## Быстрый старт (локально)

```bash
# 1. Клонировать и настроить
cp .env.example .env
# Отредактировать .env — вставить свои пароли

# 2. Запустить
make up

# 3. Создать админа в Navidrome
# Открыть http://localhost:4533 и пройти initial setup

# 4. Создать инвайт-ссылку
# Открыть http://localhost:3000/admin, пароль из AUTH_ADMIN_PASSWORD_HASH
```

## Production

```bash
# 1. Настроить .env и DNS
# 2. Убедиться, что Traefik запущен во внешней сети "web"
# 3. Запустить
make prod-up
```

## Команды

| Команда | Описание |
|---------|----------|
| `make up` | Запуск локально |
| `make down` | Остановка локально |
| `make build` | Пересборка auth-panel |
| `make logs` | Просмотр логов |
| `make hash` | Генерация bcrypt-хеша |
| `make clean` | Удаление всех данных |

## Структура

```
├── docker-compose.yml          # Production (Traefik)
├── docker-compose.local.yml    # Локальный запуск
├── .env.example                # Шаблон переменных
├── auth-panel/                 # Go-сервер авторизации
│   ├── cmd/auth-panel/         # Точка входа + шаблоны
│   ├── internal/               # Пакеты (auth, db, upload)
│   └── Dockerfile
├── data/                       # Navidrome БД
├── music/                      # Общая музыкальная библиотека
└── auth-db-data/               # PostgreSQL данные
```

## Лицензия

[AGPL-3.0](LICENSE)
