# 声学标注审定与发布服务

本项目为生态声学资料管理员、物种标注员和数据发布复核人提供一条完整、可审计的录音数据集发布流程。批次从 `draft` 开始，经过双人互盲标注、一致性计算、冲突裁决、授权与敏感人声门禁、清单冻结，最终进入 `released`。系统不包含采集调度、统计报表或外部数据门户。

服务采用 Go 标准库实现版本化 JSON HTTP API。写入由本地仓储串行提交，通过 `If-Match-Version` 执行乐观并发控制，通过 `Idempotency-Key` 安全重试。事件日志为带长度前缀、校验和、递增序号和前序摘要的只追加文件，并配有带 `schemaVersion` 的原子快照。

## 构建、运行与测试

构建服务：

```text
go build ./cmd/server
```

默认仅监听高位回环地址 `127.0.0.1:19081`：

```text
go run ./cmd/server -addr=127.0.0.1:19081 -data-dir=./data
```

也可以设置 `PORT` 为高位端口号，此时服务绑定 `127.0.0.1:<PORT>`。显式 `-addr` 优先于 `PORT`。服务拒绝裸端口、非 `127.0.0.1` 地址和常见端口。

运行全部测试：

```text
go test ./...
```

运行有界自检。该命令启动真实 TCP 监听器，通过 HTTP 完成创建批次、登记录音、两轮标注、门禁、冻结、签发、查询及凭据链校验，然后关闭服务并自行退出：

```text
go run ./cmd/server -addr=127.0.0.1:19081 -selfcheck -selfcheck-timeout=10s
```

## API 约定

业务接口位于 `/api/v1`。写请求必须携带：

- `X-Actor-ID`：操作者身份；
- `X-Role`：`administrator`、`annotator` 或 `reviewer`；
- `If-Match-Version`：期望批次版本，创建时为 `0`；
- `Idempotency-Key`：当前批次内本次操作的稳定幂等键。

JSON 写请求使用 `Content-Type: application/json`，正文上限为 1 MiB，未知字段会被拒绝。批量录音、裁决和整改每次最多 100 项，任一项目预检失败时整批不落库，错误会在 `error.details.items` 中给出数组位置、业务标识、字段和原因码。所有响应包含 `X-Request-ID`。

主要路由如下：

- `POST /api/v1/review-batches` 创建批次；
- `POST /api/v1/review-batches/{batchID}/clips` 登记录音；
- `POST /api/v1/review-batches/{batchID}/clips/bulk` 原子批量登记录音并按内容摘要防重；
- `POST /api/v1/review-batches/{batchID}/start-annotation` 生成并开始双轮盲标；
- `POST /api/v1/review-batches/{batchID}/annotations` 提交标注；
- `GET /api/v1/review-batches/{batchID}/annotation-tasks?round=1&status=pending` 查询当前标注员的盲标待办和下一修订号；
- `GET /api/v1/review-batches/{batchID}/clips/{clipID}/annotations?round=1` 查询符合互盲规则的标注；
- `GET /api/v1/review-batches/{batchID}/conflicts?status=open&reasonCode=label_mismatch` 查询按状态和原因筛选的冲突待办；
- `POST /api/v1/review-batches/{batchID}/conflicts/decisions` 原子批量裁决不同冲突项；
- `POST /api/v1/review-batches/{batchID}/conflicts/{conflictID}/decisions` 裁决或退回重标；
- `PATCH /api/v1/review-batches/{batchID}/clips/{clipID}/privacy` 整改授权与敏感人声事实；
- `GET /api/v1/review-batches/{batchID}/release-gate` 检查结构化发布阻断项；
- `POST /api/v1/review-batches/{batchID}/release-gate/remediations` 原子批量整改当前授权或静音说明阻断并立即复检；
- `POST /api/v1/review-batches/{batchID}/freeze` 原子冻结稳定清单；
- `POST /api/v1/review-batches/{batchID}/credentials` 签发发布凭据；
- `GET /api/v1/review-batches/{batchID}` 查询状态、门禁、清单、凭据与完整审计轨迹；
- `GET /api/v1/credentials/verify` 离线规则校验当前凭据摘要链；
- `GET /healthz` 健康检查。

批量录音请求使用 `{"clips":[...]}`，批量裁决使用 `{"decisions":[...]}`，门禁整改使用 `{"remediations":[...]}`。批量响应及幂等重放结果均采用录音标识或冲突标识稳定排序。退回重标的批量决定必须指定 `reannotationRound`，对应标注员按待办给出的 `nextRevision` 提交后，冲突进入 `awaiting_review`，仍需复核人再次明确裁决。

冻结后，服务拒绝修改录音、标注、裁决和门禁事实。审计只记录批量登记数量与录音标识、裁决摘要或整改阻断原因码，不记录冗长静音说明正文。发布凭据包含全局递增序号、前序凭据摘要、清单摘要、签发者与签发时间；摘要链校验会给出可定位的失败序号和原因。
