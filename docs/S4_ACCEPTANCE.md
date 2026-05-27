# LabelHub S4 Acceptance — 异步多格式导出 + Owner 数据看板 Checkpoint

> 模板:code session 实现完 S4 后逐节填写(对照 `docs/PLAN-S4-IMPL.md` 与 `PLAN.md` §Sprint4 完成标准)。
> `TODO` / 空表格是待填占位。砍掉的加分项要写明"砍 X,理由 Y"。

**完成日期**:TODO
**分支 / 关键 commit**:TODO

---

## 1. S4 已交付能力

- TODO(一句话一条,如:异步导出 4 格式 queued→worker 落盘;字段映射选列/重命名;签名下载;Owner VChart 看板 …)

## 2. 新增 / 改动 API 端点

| 方法 | 路径 | 鉴权 | 说明 |
|---|---|---|---|
| POST | `/api/v1/tasks/:taskId/exports` | owner/admin | 创建异步导出(format + field_map + include_reviews)→ queued |
| GET | `/api/v1/tasks/:taskId/exports` | owner/admin | 导出历史列表 |
| GET | `/api/v1/tasks/:taskId/exports/:exportId/download-url` | owner/admin | 返回 HMAC 签名下载 URL |
| GET | `/api/v1/exports/download?token=` | 公开(token 即鉴权) | 流式下载导出文件 |
| GET | `/api/v1/tasks/:taskId/stats` | owner/admin | 看板统计(进度/通过率/AI vs 人工/维度均分) |
| GET | `/api/v1/tasks/:taskId/export/json` | owner/admin | (S1 遗留同步 JSON,保留) |

## 3. 异步导出链路验证

- [ ] `POST /exports` 同事务写 `exports(queued)` + `outbox_events(topic=export)` + audit(queued)
- [ ] outbox publisher 入队 asynq(幂等 TaskID,重复投递不重跑)
- [ ] worker `handleExport` → `exporter.Run`:queued→running→succeeded,文件原子落盘(temp→rename)
- [ ] 失败路径:status=failed + error_msg(脱敏),前端历史显示 failed 徽章
- [ ] asynqmon(`http://localhost:8081`)可见 export 队列

## 4. 四格式文件校验记录

| 格式 | 文件 | 校验方式 | 结果 |
|---|---|---|---|
| json | TODO | `jq .` | TODO |
| jsonl | TODO | `jq -c .`(逐行) | TODO |
| csv | TODO | Excel/Numbers 打开,中文不乱码(UTF-8 BOM) | TODO |
| xlsx | TODO | Excel/Numbers 打开,表头一致 | TODO |

- [ ] 字段映射:选列 + 重命名 + "含审核记录"开关三者都在导出文件里体现

## 5. Dashboard 看板核对(数字 vs DB 直查)

| 指标 | 看板值 | DB 直查 | 一致? |
|---|---|---|---|
| 进度 finished/total | TODO | `SELECT finished_items,total_items FROM tasks WHERE id=?` | TODO |
| 状态分布 | TODO | `SELECT status,COUNT(*) FROM submissions WHERE task_id=? GROUP BY status` | TODO |
| 通过率 | TODO | approved/(approved+rejected) | TODO |
| AI vs 人工差异率 | TODO | TODO | TODO |
| 各维度均分 | TODO | (Go 侧聚合 ai_reviews.dimensions) | TODO |

- [ ] VChart 渲染正常,无 console error
- [ ] 题目预览入口(S2 已有 `item-preview`)仍可用

## 6. 分发策略验收

- [ ] `first_come` 独立可验收(必交)— 两 labeler 并发领题各拿不同 item,无重复
- [ ] (Bonus C)`assigned` 独立验收 —— TODO / 砍(理由:____)
- [ ] (Bonus C)`quota` 独立验收 —— TODO / 砍(理由:____)

## 7. 加分项状态

| 加分项 | 状态(做了/砍了) | 验收 / 砍掉理由 |
|---|---|---|
| Markdown 导出(§Bonus A) | TODO | typora 打开 + 表头一致 / 理由 ____ |
| JSONL·Excel 导入(§Bonus B) | TODO | 字段对齐 JSON 导入 / 理由 ____ |
| assigned+quota 分发(§Bonus C) | TODO | 各独立验收 / 理由 ____ |

## 8. 验收脚本(可重复)

```bash
# 1. 起环境
cd ~/Desktop/LabelHub && docker-compose -f deploy/docker-compose.yml up -d

# 2. 编译 + 单测全绿
go test ./apps/api/... ./apps/ai-worker/... ./pkg/exporter ./pkg/llmreview -count=1
pnpm -F web test && pnpm -F web lint && pnpm -F web build

# 3. seed(幂等)+ 起 API/worker(各自终端, 别后台僵尸)
docker exec labelhub-api ./tools/seed   # 或本地 make seed
make api      # 终端 1
make worker   # 终端 2

# 4. 浏览器手测(owner1/pass): 看板 → 配字段映射 → 4 格式各导一份 → 历史 succeeded → 下载校验
open http://localhost:5173
```

## 9. 已知 warning / 非阻塞遗留

- TODO(如:既有 Vite >500k chunk warning;VChart 是否 lazy-load;Bonus 砍项的后续计划)

## 10. 进入 S5 的先决条件

- [ ] `pkg/exporter` 单测覆盖充分(S5 工程质量 25% 会再加严:4 格式编码器 + Run + token 全测)
- [ ] 异步导出链路稳定,kill worker 再起能恢复 queued 任务
- [ ] 全套 `go test ./...` + `pnpm test` 全绿,`git diff --check` 干净
