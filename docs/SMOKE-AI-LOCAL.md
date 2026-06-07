# 本地 `make dev` · AI 预审 Smoke 清单

> 目的:验证**默认 seed + 默认 `.env`(mock provider)** 下,评委开箱即可走通
> Owner→Labeler→Reviewer 主链路,且 **AI 自动预审真的触发并产出 verdict**(不被模型白名单拦掉)。
> 关键被测点:seed 默认 AI Prompt 的模型 `doubao-seed-2.0-lite` 必须在 `LLM_ALLOWED_MODELS` 内,
> 否则 worker `loadReviewInput` 会报 `ai prompt model is not allowed`、AI 预审 failover 到人工。

## 0. 前置(已自动验证,无需手动)

- [x] **配置自洽**:根 `.env.example` = `LLM_PROVIDER=mock` + `LLM_ALLOWED_MODELS=doubao-seed-2.0-lite`,
      与 `apps/api/cmd/seed/main.go` 的 `defaultAIPromptModel="doubao-seed-2.0-lite"` 一致 → `AllowedModelName` 放行。
- [x] **关键链单测**:`go test ./pkg/llmreview/... ./apps/api/internal/service/aiprompt/...` = 16 passed
      (覆盖 `AllowedModelName` + mock provider `Evaluate`)。

## 1. 起本地栈

```bash
cd ~/Desktop/LabelHub
cp .env.example .env        # 注意:用「根」.env.example(mock 默认);deploy/.env.example 是 prod 模板,别混
make dev                    # = make up(mysql/redis 容器) + install + seed
# 然后三个终端各跑一个长驻进程:
make api                    # 终端 A —— :8080,启动时跑 migration
make worker                 # 终端 B —— 消费 asynq ai:review
make web                    # 终端 C —— :5173
```

预期:`make dev` 末尾打印 “Development stack is ready”;`make api` 无 `JWT_SECRET 必须设置` / `EXPORT_DIR` fatal。

## 2. Owner(登 `owner1 / 123456`,http://localhost:5173)

- [ ] 任务管理:两条官方任务 **`qa_quality`(#1)/`preference_compare`(#2)** 状态为 **发布中**。
- [ ] AI 预审页(任一任务):**AI review: 已启用**;有一条 **active prompt**;模型字段 = `doubao-seed-2.0-lite`。
- [ ] 评测集:每个任务有 **2 条 Golden Sample**(seed 写入)。
- [ ] (可选)Dry-run 测试:跑一条 → 返回 mock verdict + 维度分(证明 owner 侧 AI 通)。

## 3. Labeler(退出 → 登 `labeler1 / 123456`)

- [ ] 任务广场列出 `qa_quality` / `preference_compare`。
- [ ] 领一条 `qa_quality` → 渲染器正常显示题目(prompt / model_answer / reference;媒体题渲染 image/video/markdown)。
- [ ] 作答 → **提交**。
- [ ] **预期提交后 submission 状态 = `ai_reviewing`**(后端已建 AI plan + outbox 事件)。

## 4. AI worker(终端 B 日志 + 几秒后)

- [ ] worker 消费 `ai:review`,**无 `ai prompt model is not allowed` 报错**(← 本清单核心断言)。
- [ ] `ai_reviews` 落一行 `status=succeeded` + verdict + 维度 + overall_score + `model=doubao-seed-2.0-lite`。
- [ ] submission 据 verdict 流转:`pass→human_reviewing` / `reject→revising` / `uncertain→manual_review`。

## 5. Reviewer(退出 → 登 `reviewer1 / 123456`)

- [ ] 审核队列出现刚才那条;**AIVerdictPanel 显示 verdict + 维度 ScoreBars + `prompt v{n}`**(可见 + 可追溯到 prompt 版本/模型)。
- [ ] 选 **打回(revise)**,填 ≥5 字理由 → submission 变 `revising`。

## 6. 打回→修改闭环(回 `labeler1`)

- [ ] 「待修改」里能看到该题,顶部 banner 显示「上一轮被打回：{理由}」。
- [ ] 改答案 → 重新提交 → 状态回到 `ai_reviewing`(重新进审)。

## 7. Owner 收尾

- [ ] 审核结果页:AI-vs-人工一致性可见。
- [ ] 导出:选 JSON/JSONL/CSV/XLSX 任一 → 异步生成 → 下载,文件结构正确(逐 item 行 + answer + ai_review.* 溯源列)。

---

## 重要提醒(本地跑前必读)

1. **务必 `cp .env.example .env`(根)再 `make dev`**。若你本机已有一份 **real-doubao 的 `.env`**
   (`LLM_PROVIDER=doubao` + `LLM_ALLOWED_MODELS=${DOUBAO_EP_ID}`),seed 的 `doubao-seed-2.0-lite`
   **不在该白名单内 → AI 预审会被拦**。两种处理:① 本地演示用 mock,就 `cp .env.example .env`;
   ② 想本地用真豆包,则把 `.env` 的 `LLM_ALLOWED_MODELS` 加上 `doubao-seed-2.0-lite`(或把 seed prompt 的模型在 UI 改成你的 `ep-...`)。
2. mock provider 产出的是**确定性**的演示 verdict,不是真豆包评分——评委看到的是「链路通 + 维度分齐」,符合"可正常运行"的验收口径。
3. `make api`/`make worker`/`make web` 是前台长驻进程,**演示完 Ctrl+C 关掉**,别让它们后台堆着吃内存。
