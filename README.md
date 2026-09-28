# Self-hosted Music

Домашний музыкальный сервис на Docker Compose: Navidrome для прослушивания, MeTube/yt-dlp для загрузки, PostgreSQL и собственный Go `auth-panel` для единой авторизации и управления библиотекой.

## Возможности

- регистрация пользователей по одноразовым инвайт-ссылкам;
- единый вход через учётные записи Navidrome;
- загрузка аудиофайлов и ZIP-архивов;
- поиск, предпрослушивание и добавление треков, альбомов и плейлистов из SoundCloud;
- защищённый доступ к MeTube через `auth-panel`;
- фоновое сканирование метаданных через MusicBrainz;
- ручная проверка, редактирование и применение предложенных тегов с сохранением исходного снимка;
- управление пользователями и ролями в админ-панели;
- единый интерфейс со светлой и тёмной темой;
- Subsonic API Navidrome для совместимых мобильных и настольных приложений.

## Состав

- **Navidrome** — каталог, веб-плеер и Subsonic API;
- **MeTube** — очередь yt-dlp и загрузка аудио;
- **Auth Panel** — Go-приложение с регистрацией, загрузкой, SoundCloud-поиском, админкой и прокси к MeTube;
- **PostgreSQL** — пользователи, инвайты и состояние обработки метаданных;
- **Traefik** — внешний HTTPS reverse proxy в production;
- **Proxy bridge** — узкий мост к локальному исходящему прокси для SoundCloud, MusicBrainz и yt-dlp.

Поток данных:

```text
загрузка / SoundCloud / MeTube
              ↓
          ./music
              ↓
проверка и применение метаданных
              ↓
      индексирование Navidrome
```

## Быстрый локальный запуск

Требуются Docker и Docker Compose v2.

```bash
cp .env.example .env
# Укажите безопасные AUTH_DB_PASSWORD, NAVIDROME_ADMIN_USER
# и NAVIDROME_ADMIN_PASSWORD.
make up
```

После запуска:

1. Откройте `http://localhost:4533` и создайте первого администратора Navidrome. Его логин и пароль должны совпадать со значениями в `.env`.
2. Откройте `http://localhost:3000/login` и войдите через эту учётную запись.

Локальный `auth-panel` принимает небезопасный HTTP-origin только для `localhost` и loopback-адресов. Публичное или LAN-развёртывание должно использовать production Compose и HTTPS.

Остановка локального стека:

```bash
make down
```

## Production

1. Настройте `.env`:

```dotenv
NAVIDROME_DOMAIN=music.example.com
AUTH_PANEL_DOMAIN=music-admin.example.com
NAVIDROME_PUBLIC_URL=https://music.example.com
NAVIDROME_ADMIN_USER=admin
NAVIDROME_ADMIN_PASSWORD=change-me
AUTH_DB_PASSWORD=change-me
```

2. Убедитесь, что внешний Traefik подключён к Docker-сети `web`.
3. Запустите стек:

```bash
make prod-up
```

Production Compose не публикует внутренние порты. Navidrome и `auth-panel` доступны только через HTTPS-маршруты Traefik; MeTube и PostgreSQL остаются во внутренней сети.

## Основные маршруты Auth Panel

- `/login` — вход;
- `/register?invite=…` — регистрация по одноразовому инвайту;
- `/upload` — загрузка файлов и ZIP;
- `/discover` — поиск и добавление музыки из SoundCloud;
- `/metube/` — защищённая оболочка MeTube;
- `/admin` — инвайты, пользователи и проверка метаданных.

## Разработка и проверки

```bash
cd auth-panel
go test ./...
go vet ./...
```

Проверка обеих Compose-конфигураций без вывода секретов:

```bash
docker compose config --quiet
docker compose -f docker-compose.local.yml config --quiet
```

Ключевые каталоги:

```text
auth-panel/       исходный код Go-приложения
data/             БД и кэш Navidrome
auth-db-data/     данные PostgreSQL
music/            единая музыкальная библиотека
```

`data/`, `auth-db-data/`, `music/`, `.env` и служебные данные не входят в Git. Для переноса рабочего экземпляра их нужно резервировать отдельно.

Upstream-образы в Compose пока используют обновляемые теги (`latest` и `16-alpine`). Обновлять их следует отдельно от функциональных изменений и после smoke-теста сервиса.

## Команды

- `make up` — собрать и запустить локальный стек;
- `make down` — остановить локальный стек;
- `make build` — пересобрать локальный `auth-panel`;
- `make logs` — открыть логи локального стека;
- `make prod-up` — собрать и запустить production;
- `make prod-down` — остановить production;
- `make clean` — удалить локальные БД и музыкальные файлы.

## Лицензия

[AGPL-3.0](LICENSE)
