FROM golang:1.23-alpine AS discord-register-bot
WORKDIR /app
COPY .dockerignore ./
COPY go.* ./
RUN go mod download
COPY *.go ./
COPY handler/* ./handler/
COPY config/* ./config/
RUN chown -R 775 ./config/ 
CMD [ "go", "run", "main.go" ]
