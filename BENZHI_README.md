# BENZHI_README

基于 Go 实现的MuralMortarGate Web 项目，一款后端服务，已完整实现面向古建筑壁画保护团队的灰浆试配与施工放行系统，覆盖任务阈值、配方试板、养护检验、偏差裁决、整改复验、冻结快照、递增凭据和摘要轨迹核验，并提供原生浏览器工作台与本地校验链账本。

## 项目说明
- 项目：benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536
- 项目用途：已完整实现面向古建筑壁画保护团队的灰浆试配与施工放行系统，覆盖任务阈值、配方试板、养护检验、偏差裁决、整改复验、冻结快照、递增凭据和摘要轨迹核验，并提供原生浏览器工作台与本地校验链账本。
- Go 工具链：`golang:1.22`
- 前端工具链：原生 HTML、CSS 和 JavaScript，由 Go 服务直接提供

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...
cd /app && GOTOOLCHAIN=local go run ./cmd/mortar-gate -selfcheck -addr=127.0.0.1:19081
cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536-amd64 linux/amd64
./build_benzhi_docker.sh benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536-arm64 linux/arm64
docker run -it benzhi-project-19ba61d2-a6d6-4837-8959-30732da59536-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/mortar-gate -selfcheck -addr=127.0.0.1:19081`
