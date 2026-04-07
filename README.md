
# Start the app (dev)
1. Ensure docker is online: sudo service docker start
2. docker-compose up --build -d

# Debug db
docker exec -it "container name" psql -U chat -d chat
