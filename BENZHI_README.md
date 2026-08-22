# BENZHI_README

## 项目说明
- 项目：benzhi-project-7ae8ebe2-7227-421b-ba28-5c537adb0ad0
- 项目用途：声轨通无障碍字幕质检台提供字幕包从创建、质检、审校、修订到冻结交付的完整中文浏览器工作流。
- Go 工具链：`golang:1.22`
- 前端工具链：原生 HTML、CSS 和 JavaScript，由 Go 服务直接提供

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...
cd /app && GOTOOLCHAIN=local go run ./cmd/subtitleqc -selfcheck -addr=127.0.0.1:19137
cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh benzhi-project-7ae8ebe2-7227-421b-ba28-5c537adb0ad0-amd64 linux/amd64
./build_benzhi_docker.sh benzhi-project-7ae8ebe2-7227-421b-ba28-5c537adb0ad0-arm64 linux/arm64
docker run -it benzhi-project-7ae8ebe2-7227-421b-ba28-5c537adb0ad0-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/subtitleqc -selfcheck -addr=127.0.0.1:19137`
