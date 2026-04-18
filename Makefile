.PHONY: up down build logs clean hash prod-up prod-down restart

# Локальный запуск
up:
	mkdir -p data music auth-db-data
	docker compose -f docker-compose.local.yml up -d --build

# Остановка локального
down:
	docker compose -f docker-compose.local.yml down

# Пересборка auth-panel
build:
	docker compose -f docker-compose.local.yml up -d --build auth-panel

# Логи
logs:
	docker compose -f docker-compose.local.yml logs -f

# Перезапуск
restart:
	docker compose -f docker-compose.local.yml restart

# Production запуск
prod-up:
	docker compose up -d --build

# Production остановка
prod-down:
	docker compose down

# Генерация bcrypt-хеша
hash:
	@read -p "Password: " pass; \
	go run ./auth-panel/cmd/genhash "$$pass"

# Очистка данных (WARNING: удалит БД и файлы)
clean:
	docker compose -f docker-compose.local.yml down -v
	rm -rf data music auth-db-data
