up:
	docker-compose up --build

down:
	docker-compose down

stop:
	docker-compose stop

clean:
	docker-compose down -v

logs:
	docker-compose logs -f

sqlc:
	sqlc generate