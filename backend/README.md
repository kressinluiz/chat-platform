
# Start the app
1. Run `docker-compose up -d` to start PostgreSQL in the background.
2. go run .

# Debug db
docker exec -it backend_postgres_1 psql -U chat -d chat
