# LabelHub · 演示脚本(5/10 分钟)

> 同一份脚本两种用法:
> - **5 分钟版**(评委本机跑)— 跳过粗体 *【10 min only】* 段落
> - **10 分钟版**(录制视频/现场 demo)— 全程跑

## 录制总流程

### 1. 先定录制线路

推荐优先级:

| 线路 | 适用场景 | 地址 | 备注 |
|---|---|---|---|
| 线上 demo | 最终提交视频 | `http://43.155.210.70` | 最接近评委实际看到的环境;录制前先按 `submission/DEMO_ENV.md` 做一次线上 smoke |
| 本地 mock | 本机备用录制 / 无公网时 | `http://localhost:5173` | `cp .env.example .env && make up && make seed`,AI 预审走 deterministic mock,稳定可复现 |
| 本地真模型 | 展示真实豆包链路 | `http://localhost:5173` | 保留本机真实 `.env`,确认 `LLM_PROVIDER=doubao` 且 worker 启动无 provider 错误 |

最终视频建议只选一条主线,不要在视频里切环境。若线上状态被演示操作污染,先重跑 seed 或换 `labeler2` 录制,避免同一题已被 `labeler1` 领走导致镜头卡住。

### 2. 录制前 15 分钟检查

- [ ] 确认当前代码/部署就是要交付的版本,`git status --short` 里没有未解释的交付物改动
- [ ] 打开目标地址并硬刷新一次,确认不是旧缓存 bundle
- [ ] 三个演示账号可登录: `owner1 / 123456`, `labeler1 / 123456`, `reviewer1 / 123456`
- [ ] Owner 任务列表里 `qa_quality` / `preference_compare` 均为发布中或可演示状态
- [ ] `qa_quality` 已启用 AI 预审,有 active prompt 和 Golden Sample
- [ ] Labeler 任务广场能看到 `qa_quality`,领取后能进入作答页
- [ ] Reviewer 审核队列能打开详情页,AI 预审结论区域不报错
- [ ] Owner 导出页能创建一次 JSONL/CSV/XLSX 导出并下载
- [ ] 浏览器缩放 100%,窗口 1920×1080,关闭 devtools、书签栏和无关 tab
- [ ] 开启勿扰模式,隐藏桌面通知、菜单栏敏感信息和输入法候选窗
- [ ] 录屏软件设置为 1920×1080 / 30fps / H.264 MP4,鼠标光标可见
- [ ] 麦克风试录 10 秒,确认音量不过曝、键盘声不过大

### 3. 本地录制启动命令

线上录制不需要跑本地服务。若选择本地线路,按下面顺序启动:

```bash
cd ~/Desktop/LabelHub
cp .env.example .env
make up
make seed
make api
make worker
make web
```

`make api` / `make worker` / `make web` 建议分三个终端前台运行,方便录制前确认没有启动错误。若要用真实豆包,不要覆盖已有 `.env`;先备份再手动确认 `LLM_PROVIDER`、`LLM_API_KEY`、`LLM_BASE_URL`、`LLM_MODEL`。

### 4. 视频结构

| 段落 | 时长 | 目标 |
|---|---:|---|
| 开场 | 0:00-0:30 | 一句话讲清 LabelHub 是三角色数据标注平台 |
| Owner | 0:30-3:00 | 展示任务、Designer、AI Prompt、Golden Sample、Stats |
| Labeler | 3:00-5:00 | 领取题目、作答、LLM 辅助、提交触发 AI 预审 |
| Reviewer | 5:00-7:00 | 查看 AI verdict、审计、人工通过 |
| Export | 7:00-8:30 | 回 Owner 异步导出并下载 |
| 收尾 | 8:30-9:00 | 回扣工程亮点和提交资料 |

5 分钟版压缩方法:Designer 只展示不拖拽,Golden Sample 只看已有结果,Labeler 不刷新验证 autosave,导出只跑 JSONL。

## 录制前准备(作者填)

- [ ] 录制日期:
- [ ] 录制线路:线上 demo / 本地 mock / 本地真模型
- [ ] 目标地址:
- [ ] 录制版本/commit:
- [ ] 视频目标时长:5 min / 10 min
- [ ] 录屏工具:OBS / Screen Studio / macOS 系统录屏 / 其他
- [ ] 备注:

## 录制时口播大纲(中文)

---

### Scene 0 · 开场(0:00 - 0:30) · 30s

> "大家好,我是 Zhang Youchen。这是字节 AI 全栈课题项目 **LabelHub**——一个覆盖 数据生产 → AI 预审 → 人工审核 → 多格式导出 全生命周期的数据标注平台。
>
> 技术栈:**前端 React 18 + TypeScript strict + Semi Design**,**后端 Go + Gin + GORM**,**AI Worker 走 Asynq + 豆包 Function Calling**。今天我会跑三个角色的完整链路,展示 9 + 2 个 Designer 物料、AI 预审、Stats Board 和多格式导出。"

**镜头**:浏览器首屏(登录页),录屏左下角不要遮挡 URL。

**操作**:
- 打开目标地址
- 确认登录页完整显示
- 鼠标停在角色快捷登录或用户名输入框附近,不要快速晃动

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

> "AI Prompt 本地默认走 deterministic mock,配置豆包后可切到真实 Function Calling。Go 端严格校验 verdict/score/dimensions schema。Golden Sample 是 dry-run 沙盒——评委可以提前知道 prompt 在已知答案上的表现。这里看 dry-run 结果与 expected verdict 匹配。"

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
- 若队列需要几秒,停留在页面上口播 outbox/worker 链路,不要连续乱点

> "提交后业务事务同写 outbox 表,后台 publisher 用 SELECT FOR UPDATE SKIP LOCKED 把事件投到 Redis,AI Worker 拉走跑豆包。整个链路是 durable outbox 加至少一次投递,消费端保证幂等。"

---

### Scene 3 · Reviewer — 审核 + AI Verdict(5:00 - 7:00) · 2 min

#### 3.1 切角色 + 审核队列(5:00 - 5:30) · 30s
- 退出 → 登 `reviewer1` / `123456`
- 进入审核队列,等待刚才那条出现(AI 已跑完)
- 若刚才提交未出现,刷新一次队列;仍未出现就打开已有 demo submission,口播说明这是同一条 AI 预审链路的历史样例

> "Reviewer 看到 AI 预审结果——verdict、score、三维度评分、处理日志和 prompt 版本号。"

#### 3.2 详情 + AI verdict(5:30 - 6:30) · 60s
- 点条目进详情
- 左侧:Labeler 答案 + ShowItem 上下文
- 右侧:AI verdict(pass/reject/uncertain)+ 三维度评分柱 + reason 文本 + tokens/latency 元数据
- *【10 min only】* 演示右侧 **规则** tab,切换历史 prompt 版本看 dry-run 历史

> "AI verdict 是辅助而非自动决策。AI pass 进入人工初审,AI uncertain 进入人工复核,AI reject 会直接打回标注员修改;最终入库仍由人工终审决定。这里我手动通过。"

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

- [ ] 看一遍完整视频,确认没有密码管理器弹窗、聊天通知、API key、终端密钥或私人路径
- [ ] 剪掉开头等待、结尾空白和明显误点;不要剪掉关键加载过程,保留平台真实感
- [ ] 音频响度大致一致,无长时间静音
- [ ] 字幕可选;若加字幕,只修明显口误,不要改成视频里没展示的能力
- [ ] 视频导出为最终提交附件:MP4 / H.264 / 1080p / 30fps,不放入 Git 仓库
- [ ] 推荐命名:`demo-5min.mp4` 或 `demo-10min.mp4`
- [ ] 压缩后文件大小符合提交平台限制,播放无花屏
- [ ] 如界面有变化,刷新 4 个关键节点截图(Owner Designer / Labeler 答题 / Reviewer 详情 / 导出 history)到 `assets/screenshots/`
- [ ] 在本文件顶部填写 "录制日期 / 视频时长 / 备注"
- [ ] 上传视频到比赛平台或外部附件,并在最终提交说明里放链接/附件名

## 备用素材路径(若现场出问题)

| 兜底场景 | 切到 |
|---|---|
| AI worker 没跑出来 | `LLM_PROVIDER=mock` 默认走 deterministic mock,不应该失败;若失败展示 reviewer 直接审核(skip AI) |
| MySQL 连不上 | 重跑 `make down && make up && make seed` |
| 前端白屏 | F12 → Network tab,通常是 :8080 被占,改 API_PORT |
| Designer 拖拽失灵 | 切到 1920 视口(响应式 ≤1599 时拖拽手柄藏起来) |
| 刚提交的题没进 Reviewer 队列 | 先刷新队列;仍没有就切已有 demo submission,录完后用 smoke 脚本补查 AI worker |
| 线上任务被误下线/数据污染 | 重跑 seed 或切本地 mock 线路,视频里不要现场修库 |
