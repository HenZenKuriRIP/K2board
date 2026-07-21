# K2Board Docker 镜像
# 构建: docker build -f deploy/Dockerfile -t k2board .
# 运行: docker run -d -p 8080:8080 -v $(pwd)/config.yml:/app/config.yml -v k2board-data:/app/data k2board

FROM golang:1.26-alpine AS builder
RUN apk add --no-cache gcc musl-dev nodejs npm
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN cd web && npm install && npm run build
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o k2board ./cmd/server

FROM alpine:latest
RUN apk add --no-cache ca-certificates tzdata
ENV TZ=Asia/Shanghai
WORKDIR /app
COPY --from=builder /src/k2board .
EXPOSE 8080
VOLUME ["/app/data"]
CMD ["./k2board"]
