COMPOSE := $(shell command -v docker-compose 2>/dev/null || echo 'docker compose')

up:
	$(COMPOSE) up --build

down:
	$(COMPOSE) down

stop:
	$(COMPOSE) stop

clean:
	$(COMPOSE) down -v

logs:
	$(COMPOSE) logs -f

sqlc:
	sqlc generate