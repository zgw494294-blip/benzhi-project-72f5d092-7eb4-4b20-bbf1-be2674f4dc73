# BENZHI_README

基于 Go 实现的acoustic-annotation-release HTTP API 项目，一款后端服务，已完整实现生态声学录音从批次创建、双人互盲标注、冲突裁决、发布门禁、清单冻结到摘要链凭据签发与校验的版本化 JSON HTTP 服务，并提供可恢复本地持久化、审计轨迹和真实回环监听自检。

## 项目说明
- 项目：benzhi-project-72f5d092-7eb4-4b20-bbf1-be2674f4dc73
- 项目用途：已完整实现生态声学录音从批次创建、双人互盲标注、冲突裁决、发布门禁、清单冻结到摘要链凭据签发与校验的版本化 JSON HTTP 服务，并提供可恢复本地持久化、审计轨迹和真实回环监听自检。
- Go 工具链：`golang:1.22`
- 前端工具链：无

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...
cd /app && GOTOOLCHAIN=local go run ./cmd/server -addr=127.0.0.1:19081 -selfcheck -selfcheck-timeout=10s
cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh benzhi-project-72f5d092-7eb4-4b20-bbf1-be2674f4dc73-amd64 linux/amd64
./build_benzhi_docker.sh benzhi-project-72f5d092-7eb4-4b20-bbf1-be2674f4dc73-arm64 linux/arm64
docker run -it benzhi-project-72f5d092-7eb4-4b20-bbf1-be2674f4dc73-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/server -addr=127.0.0.1:19081 -selfcheck -selfcheck-timeout=10s`
