
# Start the app
1. docker-compose up --build -d

# Debug db
docker exec -it "container name" psql -U chat -d chat
