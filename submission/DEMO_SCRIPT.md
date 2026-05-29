# LabelHub · 演示脚本(5/10 分钟)

> 同一份脚本两种用法:
> - **5 分钟版**(评委本机跑)— 跳过粗体 *【10 min only】* 段落
> - **10 分钟版**(录制视频/现场 demo)— 全程跑

## 录制前准备(作者填)

- [ ] 确认仓库在 `main`(`git status` 干净)
- [ ] `cp .env.example .env`
- [ ] `make dev` 起栈,等到 "Development stack is ready" 提示
- [ ] 三终端起好:`make api` / `make worker` / `make web`
- [ ] http://localhost:5173 已加载首屏
- [ ] 浏览器开发者面板关闭 / 缩放 100%(避免视频里看到 devtools)
- [ ] 录屏分辨率 1920×1080,鼠标光标可见
- [ ] 准备好 OBS / Screen Studio / macOS 系统录屏

## 录制时口播大纲(中文)

---

### Scene 0 · 开场(0:00 - 0:30) · 30s

> "大家好,我是 Zhang Youchen。这是字节 AI 全栈课题项目 **LabelHub**——一个覆盖 数据生产 → AI 预审 → 人工审核 → 多格式导出 全生命周期的数据标注平台。
>
> 技术栈:**前端 React 18 + TypeScript strict + Semi Design**,**后端 Go + Gin + GORM**,**AI Worker 走 Asynq + 豆包 Function Calling**。今天我会跑三个角色的完整链路,展示 9 + 2 个 Designer 物料、AI 预审、Stats Board 和多格式导出。"

**镜头**:浏览器首屏(登录页),录屏左下角不要遮挡 URL。

---

### Scene 1 · Owner — 任务/模板/AI Prompt(0:30 - 3:00) · 2.5 min

#### 1.1 登录(0:30 - 0:45) · 15s
- 登 `owner1` / `123456`
- 进入 Owner Dashboard,左侧选 **官方任务 qa_quality**

> "Owner 是任务发布者。Seed 里有两个官方任务——qa_quality 30 条文本质检题,preference_compare 12 条 A/B 偏好题。我们先看第一个。"

#### 1.2 模板 Designer(0:45 - 1:30) · 45s
- 点 **模板** tab → 进 Designer
- *【10 min only】* 演示拖拽:从左侧物料拖一个 Radio 到画布,改 label/name/options
- 滚到 Group / Tabs 嵌套字段,展示画布预览(子字段以迷你行展示,Tabs 切换)
- 切窗口宽度 1920 ↔ 1280,展示响应式(三栏 ↔ 属性面板下移)

> "Designer 支持 9 个核心物料 + Group/Tabs 2 个加分物料。Tabs 是真实的 ARIA tablist,切换时已填答案保留。响应式断点在 1920 / 1280 / 768。"

#### 1.3 AI Prompt + Golden Sample(1:30 - 2:30) · 60s
- 切到 **AI Prompt** 区,展示三维度 + threshold slider
- 切到 **Golden Sample** 区,选一条样本点 **Run dry-run**
- 弹出 history / trend,展示 matched / mismatch 对比

> "AI Prompt 走豆包 Function Calling,Go 端严格校验 verdict/score/dimensions schema。Golden Sample 是 dry-run 沙盒——评委可以提前知道 prompt 在已知答案上的表现。这里看 dry-run 结果与 expected verdict 匹配。"

#### 1.4 Stats Board(2:30 - 3:00) · 30s
- 切到 **Stats** tab,展示进度 / 通过率 / AI vs 人工 / 维度均分 chart

> "Stats Board 是 VChart 异步加载,首屏不耗 2.2MB,只在打开时按需 412KB。"

---

### Scene 2 · Labeler — 任务广场 → 提交(3:00 - 5:00) · 2 min

#### 2.1 切角色(3:00 - 3:15) · 15s
- 退出 → 登 `labeler1` / `123456`
- 进入 **任务广场**

> "Labeler 是标注员。任务广场按 first-come 抢占,我领取 qa_quality 第一题。"

#### 2.2 答题(3:15 - 4:30) · 75s
- 看左侧 ShowItem 渲染(text / video / image / markdown 自适应)
- 填中间答题区(Radio / Tags / TextArea / RichText)
- *【10 min only】* 演示草稿自动保存:答几个字 → 等 3 秒 → 刷新页面 → 答案还在
- 触发 **LLM 辅助按钮**(LLMTrigger),AI 回填 `target_field`

> "ShowItem 自动识别 payload 类型。这里 LLM 触发是 Labeler 自助拉 AI 草稿,降人工成本——结果通过 Function Calling 回填到指定字段。"

#### 2.3 提交(4:30 - 5:00) · 30s
- 按 **Ctrl/Cmd + Enter** 提交(展示快捷键)
- 等 toast 提示 "已提交,AI 预审中"

> "提交后业务事务同写 outbox 表,后台 publisher 用 SELECT FOR UPDATE SKIP LOCKED 把事件投到 Redis,AI Worker 拉走跑豆包。整个链路保证恰好一次。"

---

### Scene 3 · Reviewer — 审核 + AI Verdict(5:00 - 7:00) · 2 min

#### 3.1 切角色 + 审核队列(5:00 - 5:30) · 30s
- 退出 → 登 `reviewer1` / `123456`
- 进入审核队列,等待刚才那条出现(AI 已跑完)

> "Reviewer 看到 AI 预审结果——verdict、score、三维度评分、处理日志和 prompt 版本号。"

#### 3.2 详情 + AI verdict(5:30 - 6:30) · 60s
- 点条目进详情
- 左侧:Labeler 答案 + ShowItem 上下文
- 右侧:AI verdict(pass/reject/uncertain)+ 三维度评分柱 + reason 文本 + tokens/latency 元数据
- *【10 min only】* 演示右侧 **规则** tab,切换历史 prompt 版本看 dry-run 历史

> "AI verdict 是辅助而非自动决策——除非 Owner 在 AI Prompt 启用 'pass 自动 approved' 才会跳过人工。这里我手动通过。"

#### 3.3 通过(6:30 - 7:00) · 30s
- 点 **通过**,review 事务内双锁 + RowsAffected 守状态机
- *【10 min only】* 演示批量:选 3 条点批量通过

> "审核动作触发 review.Apply,事务内 FOR UPDATE 锁 task 和 submission,RowsAffected != 1 直接 ErrConcurrentWrite——保证两个 reviewer 同时点不会双写。"

---

### Scene 4 · 导出 + 收尾(7:00 - 8:30 / 9:30) · 1.5-2.5 min

#### 4.1 回 Owner 导出(7:00 - 8:00) · 60s
- 退出 → 登 `owner1`
- 进任务详情 → **导出** tab
- 选 **JSONL** 格式 → 选导出字段映射 → 点 **开始导出**
- history 出现一行 queued → running → succeeded
- 点 **下载**,文件落本地

> "导出走 outbox + worker 异步:queued 写库 + 入队,worker 按 task ID 跑 exporter,原子 temp → rename,下载走 HMAC 签名 URL,默认 10 分钟过期。"

#### 4.2 *【10 min only】* 多格式对比(8:00 - 9:00) · 60s
- 再导一份 CSV(展示防 Excel 公式注入,`= + - @` 开头加单引号)
- 再导 XLSX,打开看维度均分 sheet

#### 4.3 收尾(8:30 / 9:30 - 9:00 / 10:00) · 30s
- 切回 `owner1` Stats Board,展示刚才那条提交后通过率上升

> "全栈链路演示完毕。整套架构、状态机、Outbox、AI 幂等、测试纪律和首屏优化的细节,都在仓库 docs/ARCHITECTURE.md 和 README 里。CI 在每次 push 跑 Go workspace + web test + lint + build + 独立的 testcontainers 集成测试 job。
>
> 感谢评委的时间。"

---

## 录制后

- [ ] 视频导出为 `assets/demo.mp4`(MP4 / H.264 / 1080p / 30fps)
- [ ] 5min 与 10min 各导一份(`demo-5min.mp4` / `demo-10min.mp4`)
- [ ] 在 4 个关键节点截图(Owner Designer / Labeler 答题 / Reviewer 详情 / 导出 history)放到 `assets/screenshots/`
- [ ] 在本文件顶部追加 "录制日期 / 视频时长 / 备注"

## 备用素材路径(若现场出问题)

| 兜底场景 | 切到 |
|---|---|
| AI worker 没跑出来 | `LLM_PROVIDER=mock` 默认走 deterministic mock,不应该失败;若失败展示 reviewer 直接审核(skip AI) |
| MySQL 连不上 | 重跑 `make down && make up && make seed` |
| 前端白屏 | F12 → Network tab,通常是 :8080 被占,改 API_PORT |
| Designer 拖拽失灵 | 切到 1920 视口(响应式 ≤1599 时拖拽手柄藏起来) |
