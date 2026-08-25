# MuralMortarGate 壁画修复灰浆放行台

MuralMortarGate 面向古建筑壁画保护团队，将灰浆试配任务、配方与试板登记、分阶段养护检验、偏差专业裁决、整改复验、施工批次冻结和放行凭据签发收束为一条可追溯流程。系统的核心目标是阻止未经验证的材料直接用于珍贵壁画。

浏览器工作台、JSON API 和静态资源均由同一个 Go 服务提供，不依赖 Node 或外部业务系统。打开 `http://127.0.0.1:19081/workbench` 即可完成主要流程。

## 业务状态与证据约束

任务创建后处于 `draft`；登记配方和试板后进入 `curing`。既可登记新配方，也可引用当前任务内的既有配方一次登记最多 20 块平行试板；整组编号、养护开始时间和严格递增节点先统一校验，随后只递增一次版本。检验工作台按计划时间显示未到期、今日到期、逾期和已确认节点，并可一次原子确认最多 50 块试板的当前节点。已确认的原始测量不能覆盖；已有完整通过候选时进入 `pending_approval`，其他未完成试板仍可继续确认节点。

要求整改的任务进入 `remediation`。整改方案必须关联当前轮具体偏差并记录措施、期限和责任人；每次格式合法的复验均作为新轮次追加。复验失败也会保存四项判定、结束当前方案并生成带来源复验编号的下一轮偏差，必须登记新方案后才能再次复验。所有活动偏差闭环后任务恢复 `pending_approval`。

任务详情按实际配方与所属试板生成冻结预检矩阵，列出节点证据、四项判定、偏差闭环、复验链、试板结论和职责分离的结构化检查。可冻结候选会生成绑定任务版本、配方、试板和批准人的 `eligibilityDigest`；冻结请求必须携带该摘要，服务在任务锁内重算一致后才把摘要及通过项固化进不可变批次快照。签发凭据后状态变为 `released`。

每个写请求包含 `expectedVersion` 与长度为 8 至 128 字符的 `idempotencyKey`。任务级互斥负责串行化写入，版本号拒绝陈旧修改，同一幂等键只能重放完全相同的请求。

## 构建、运行与测试

标准构建：

```bash
go build ./...
```

标准测试：

```bash
go test ./...
```

默认运行：

```bash
go run ./cmd/mortar-gate
```

默认监听 `127.0.0.1:19081`。可显式指定其他回环高位端口：

```bash
go run ./cmd/mortar-gate -addr=127.0.0.1:19181
```

未显式传入 `-addr` 时，也可使用 `PORT` 端口号，服务将绑定 `127.0.0.1:<PORT>`；显式 `-addr` 的优先级更高。服务拒绝非回环地址和低于 `1024` 的端口。

执行会自行退出的真实 HTTP 自检：

```bash
go run ./cmd/mortar-gate -selfcheck -addr=127.0.0.1:19081
```

自检使用临时数据目录，启动真实回环监听器，通过 JSON API 完成一次包含失败整改的完整流程，查询凭据并重新计算内容摘要，然后优雅关闭服务。

## 本地持久化

正常运行默认使用 `data/`，可通过 `-data-dir` 修改。目录包含：

- `events.frames`：长度前缀 JSON 事件帧。每帧含 `schemaVersion`、递增序号、前序摘要和自身校验摘要，确认前执行文件同步。启动时会截断未完成尾帧，但拒绝中段损坏、序号断裂或摘要链错误。
- `projection.json`：带 `schemaVersion` 的任务与幂等投影。服务从事件日志重放并核验投影，通过临时文件写入、同步和原子替换发布。

放行凭据包含递增 `credentialNo`、冻结批次快照、签发时间、前一凭据摘要和当前内容摘要。`GET /api/v1/credentials/{credentialNo}` 会重新计算摘要并返回完整任务状态轨迹和账本完整性信息。

## 主要接口

- `GET /workbench`：浏览器工作台。
- `GET /healthz`：健康检查。
- `POST /api/v1/trials`：创建试配任务。
- `GET /api/v1/trials/{id}`：读取任务、证据矩阵和可用动作。
- `POST /api/v1/trials/{id}/panels`：登记新配方与试板，或用 `formulaId` 引用既有配方并通过 `panels` 整组登记平行试板；兼容原 `panel` 单条字段。
- `POST /api/v1/trials/{id}/measurements`：通过 `measurements` 原子确认多块试板的当前到期节点；兼容原 `panelId` 与 `measurement` 单条字段。
- `POST /api/v1/trials/{id}/reviews`：提交专业偏差裁决。
- `POST /api/v1/trials/{id}/remediations`：登记限期整改方案。
- `POST /api/v1/trials/{id}/retests`：追加整改复验轮次。
- `POST /api/v1/trials/{id}/freeze`：携带任务详情候选返回的 `eligibilityDigest`，复检一致后冻结施工批次。
- `POST /api/v1/trials/{id}/release`：签发放行凭据。
- `GET /api/v1/credentials/{credentialNo}`：校验凭据摘要与状态轨迹。
