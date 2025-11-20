up:
	docker-compose up --build

down:
	docker-compose down

stop:
	docker-compose stop

logs:
	docker-compose logs -f

sqlc:
	sqlc generate