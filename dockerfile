FROM golang:1.25.6-alpine AS builder

WORKDIR /app

# Установка зависимостей, нужных для сборки (включая gcc для CGO)
RUN apk add --no-cache ca-certificates tzdata gcc musl-dev sqlite-dev

# Копирование go модулей
COPY go.mod go.sum ./
RUN go mod download

# Копирование всех исходных файлов (кроме игнорируемых в .dockerignore)
COPY . .

# Сборка бинарного файла
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o main .

# Финальный этап
FROM alpine:latest

# Установка зависимостей для запуска (sqlite и сертификаты)
RUN apk --no-cache add ca-certificates tzdata sqlite-dev

WORKDIR /root/

# Копирование бинарного файла
COPY --from=builder /app/main .

# Создание точки монтирования для базы данных и конфигураций
VOLUME ["/root/data"]

# Команда запуска
CMD ["./main"]
