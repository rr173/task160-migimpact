FROM docker.m.daocloud.io/library/golang:1.26.3-bookworm

WORKDIR /app

# 先复制依赖文件并下载依赖，利用 Docker 缓存并保证容器内可用
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build ./...

CMD ["bash"]
