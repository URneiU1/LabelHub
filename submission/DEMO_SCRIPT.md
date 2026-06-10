# LabelHub · 演示脚本(5/10 分钟)

> 同一份脚本两种用法:
> - **5 分钟版**(评委本机跑)— 跳过粗体 *【10 min only】* 段落
> - **10 分钟版**(录制视频/现场 demo)— 全程跑

---

## ⭐ 从零录制完整流程(线上 demo · 推荐主路径)

> 线上环境 `http://43.155.210.70` 已被多轮测试用过,数据是"住过人"的脏状态(labeler1 有 draft、preference_compare 已被认领、存在空的测试任务、AI 预审尚未在真实提交上验证过)。**直接照口播大纲录会卡镜头**。本节给出一条从干净状态起步、可一次成片的完整路径。三段:**重置 → 录前 smoke → 正式录**。

### Phase 0 · 重置 prod 到干净状态(不上镜,约 3 分钟)

幂等 `seed` 只"补建不存在的",**清不掉已有的 draft / 认领 / 测试任务**。真正干净要清空数据卷。用 `up -d`(复用现有镜像、**不加 `--build`**),避免重新编译失败导致线上下机。

```bash
ssh -i ~/Downloads/labelhub.pem ubuntu@43.155.210.70
cd /home/ubuntu/labelhub
DC="docker compose --env-file deploy/.env -f deploy/docker-compose.prod.yml"

# (可选)先备份当前库
$DC exec -T mysql sh -c 'mysqldump -uroot -p"$MYSQL_ROOT_PASSWORD" "$MYSQL_DATABASE"' > ~/labelhub-backup-before-demo.sql

$DC down -v                       # 清空 mysql/redis/exports 数据卷
$DC up -d --wait --wait-timeout 150   # 复用现有镜像重起并等健康;api 启动自动迁移建表

# 注:2026-06-10 起 prod 的 api 镜像已自带 tools/seed 数据集(Dockerfile 已加
#    COPY tools/seed /tools/seed),下面这行 seed 直接成功。
#    ——若你跑的是更早的旧镜像(seed 报 "标注要求.md: no such file or directory"),
#      先补这两行把 host 数据集拷进容器再 seed:
#        $DC exec -u root -T api mkdir -p /tools
#        $DC cp tools/seed api:/tools/seed
$DC exec -e SEED_ALLOW_IN_PROD=true -T api seed   # 写入 2 个官方任务 + 9 个账号
curl -s http://localhost/health                   # → ok

# 📌 本次 demo 用单任务设计:seed 会建 qa_quality(id=1) + preference_compare(id=2),
#    但正式录里要「现场新建第 2 个任务」演示创建,所以先删掉 seed 的 preference_compare,
#    让列表初始只剩 qa_quality(任务没有删除接口,只能 SQL 删;无外键约束,删依赖行即可):
$DC exec -T mysql sh -c 'mysql -uroot -p"$MYSQL_ROOT_PASSWORD" "$MYSQL_DATABASE" -e "
DELETE FROM acceptance_batches WHERE task_id=2; DELETE FROM ai_dry_runs WHERE task_id=2;
DELETE FROM ai_prompt_configs WHERE task_id=2; DELETE FROM exports WHERE task_id=2;
DELETE FROM golden_samples WHERE task_id=2; DELETE FROM submissions WHERE task_id=2;
DELETE FROM task_assignees WHERE task_id=2; DELETE FROM task_items WHERE task_id=2;
DELETE FROM task_reviewers WHERE task_id=2; DELETE FROM task_templates WHERE task_id=2;
DELETE FROM uploaded_files WHERE task_id=2; DELETE FROM tasks WHERE id=2;
SELECT id,title,status FROM tasks;"'   # 应只剩 1 行 qa_quality
```

> 🐞 **已知部署坑(根因 + 已修复)**:历史 `apps/api/Dockerfile` 只 COPY 了 binary + migration,**没 COPY `tools/seed/` 数据集**,所以容器内 `seed` 找不到 baseline/template 文件 → `down -v` 重置后账号能 seed 但 2 个官方任务 seed 失败。
> **2026-06-10 已永久修复并部署**:Dockerfile 最终 stage 加了 `COPY tools/seed /tools/seed`,prod 的 api 镜像已重 build,`/tools/seed` 进镜像。现在 `down -v` 重置后**直接 `seed` 即可**,无需任何 cp 绕过。上面注释里的 `mkdir /tools`+`docker cp` 只对**未重 build 的旧镜像**才需要。

**重置 + 删 preference_compare 后的干净基线(单任务 demo)**:列表只剩 `qa_quality`(30 题全可领) / 无空任务 / 所有账号无 draft 无认领 / 审核队列为空 / AI 真豆包可用。正式录里在镜头上现场新建第 2 个任务。

### Phase 1 · 录前 smoke 验 AI 预审(不上镜,约 3 分钟)

**这一步决定 reviewer 场能不能录** —— 必须确认提交后 AI Worker 真的产出 verdict。

> 📌 **本次 demo 用单任务设计**:线上只保留 `qa_quality`(富任务,跑全链路),`preference_compare` 已删(见 Phase 0 注),正式录里在镜头上**现场新建第 2 个任务**演示「创建→发布」。因为 `qa_quality` 是 `first_come`(整体独占领取),**没有第二个任务给 smoke 用**——所以:
>
> - **AI 预审已于 2026-06-10 实测可用**(真豆包,sub 提交后 ~3s 出 `pass`/维度分)。正常情况**可跳过单独 smoke,直接录**:Scene 2 里 labeler1 提交的第一条就是 live 验证,Scene 3 reviewer 就能看到 verdict。
> - 想录前再保险一次:用 `labeler1` 在 `qa_quality` 上真跑一遍(领题→提交→reviewer 看 verdict),**然后 Phase 0 重置 + 重删 preference_compare** 回到全新态再正式录(因 first_come 独占,smoke 会占掉 qa_quality)。

判定 AI 是否 fire(任何时候提交完一条后):
   - ✅ reviewer 审核队列/AI 预审队列里出现刚才那条、带 AI verdict(pass/reject/uncertain)+ 维度评分 → AI 管线 OK。
   - ❌ 一直不出现 verdict → AI 没 fire。排查:`$DC logs --tail=80 worker` 看报错。`llm provider returned HTTP 401` = key 认证失败;`404` = `LLM_MODEL`/endpoint 不存在。
     - **最常见的坑(真实踩过)**:401 不一定是 key 本身坏。改完 `deploy/.env` 后**必须用 `--force-recreate` 重启 worker**,否则 worker 还在用旧内存里的旧 key 跑、永远 401:
       ```bash
       $DC up -d --force-recreate worker     # 强制重读 .env;光 up -d worker 不会重载改过的 env
       $DC logs --tail=15 worker             # 看到 "AI Worker started" 且无 provider 报错即可
       ```
     - 真豆包修不好,**兜底切 mock**:`sed -i 's/^LLM_PROVIDER=.*/LLM_PROVIDER=mock/' deploy/.env` → `$DC up -d --force-recreate worker` → deterministic mock 必出 verdict,适合稳定录制。

> ⚠️ **改任何 `deploy/.env` 后重启 worker 一律加 `--force-recreate`** —— compose 不重建容器就不会重载 env,这是本项目最坑的一处。

### Phase 2 · 正式录制(上镜)· 用 `qa_quality` + 全新 `labeler1`

按下方《录制时口播大纲》Scene 0→4 走,落到具体数据(线上只有 `qa_quality` 一个富任务):

| Scene | 账号 | 任务 | 是否改数据 |
|---|---|---|---|
| 0 开场 | — | — | 否 |
| 1a Owner 展示 | `owner1` | `qa_quality` | 否(Designer Tabs/Group + AI Prompt/Golden Sample/Stats 只读展示「深度」) |
| **1b Owner 创建任务** | `owner1` | **现场新建第 2 个** | **新建草稿→Designer 建模板→导入官方数据集→发布**(演示完整数据生产生命周期;详细分步见表下) |
| 2 Labeler | `labeler1` | `qa_quality` | 领题+提交(全新,first_come 整体领) |
| 3 Reviewer | `reviewer1` | 刚提交那条 | 通过(或录 **打回→修订→复审** 闭环展示状态机) |
| 4 Export | `owner1` | `qa_quality` | 异步导出+下载 |

- **Scene 1b「创建 → 建模板 → 导入官方数据 → 发布」逐步操作(~3-4 min,必录,点哪个按钮都标了)**

  > ⚠️ **顺序是死的**:导入只能在**草稿**任务上做(发布后禁导入);发布又**必须先有绑定模板**。所以固定走:创建草稿 → 建模板 → 导入(仍草稿)→ 发布。顺序反了会被拦。

  以 `owner1` 登录,owner 左栏分节切换来配置任务:

  1. **创建任务**(左栏「任务管理」):点右上角 **`+ 新建任务`** → 抽屉里填「**任务标题**」(必填,如 `demo·商品标题清洗`)+「任务简介」(可选)→「分发策略」选 **`先到先得`** → 点底部 **`创建草稿`** → Toast「任务已创建为草稿」(任务自动选中)。
  2. **建模板**(左栏切「**模板搭建**」,发布前必须):点 **`打开 Designer`** → 模板列表页点 **`+ 新建模板`** → 进 Designer(显示「新建模板」)→ 从左侧物料面板**拖** 2-3 个物料(如 Radio / Tags / Input)到中间画布,右侧属性面板改 label / name → 点 **`保存并发布版本 r1`**(画布至少 1 个物料才能点)。
  3. **导入官方数据**(左栏切「**数据集**」,仍是草稿):在「文件导入(.json / .jsonl / .xlsx)」处点文件选择框 → 选 `tools/seed/datasets/qa_quality/excel/qa_quality.xlsx`(仓库根相对路径;**选中文件即自动上传**,无需额外按钮)→ Toast「**已导入 N 条(xlsx)**」→ 可点 **`随机预览`** 展示一条导入题目的 payload。
  4. **发布**(左栏切回「任务管理」):点列表里这个任务的行选中它 → 下方动作栏点 **`发布`** 按钮 → 状态徽章 `草稿 → 发布中` ✓。(没建模板就点发布会报「未绑定模板」——所以第 2 步必须先做。)

  > 口播:"Owner 是数据生产者。我现场建一个任务——填基本信息、用可视化 Designer 拖出标注表单、**导入官方数据集**、一键发布。草稿→发布中 整个生命周期由状态机驱动。"
  >
  > 分工:`qa_quality`(现成富任务)→ 展示 Designer 高级物料 + AI 配置 + labeler/reviewer/导出 全链路;**这个新建任务** → 展示「从零创建 → 建模板 → 导入 → 发布」。互补不冗余。
  >
  > 导入格式备选:`excel/qa_quality.xlsx`(最直观)/ `json/qa_quality.json`(与现成 qa_quality 题目完全一致)/ `jsonl/qa_quality.jsonl`,都在 `tools/seed/datasets/qa_quality/`,按扩展名自动识别、无需手动列映射。
- **强烈建议**:Scene 3 录一遍 **AI reject 或人工打回 → labeler 修订重提 → reviewer 复审通过** 的闭环 —— 这是最能体现「长链路工作流状态机」考察点的镜头(`submitted→ai_reviewing→…→revising→submitted→…→approved`)。

### Phase 3 · 录后

按本文末尾《录制后》清单走;若录制中任一界面与现有截图不一致,顺手刷新 `assets/screenshots/` 4 个关键截图。

### 一次性失败兜底

| 现象 | 处理 |
|---|---|
| labeler1 进作答页自动恢复了旧 draft | 没重置干净 —— 回 Phase 0 重跑 `down -v` |
| reviewer 队列里 AI verdict 不出现 | 回 Phase 1 排查(多半是改了 .env 没 `--force-recreate` 重启 worker → 旧 key 401);真豆包不稳就切 `LLM_PROVIDER=mock` + `$DC up -d --force-recreate worker` 再录 |
| 任务广场冒出空的「商品标题清洗」 | 没重置干净(那是测试残留),`down -v` 后只剩 2 个官方任务 |
| 录到一半数据乱了 | 不要现场修库;停录 → Phase 0 重置 → 重录 |

---

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
cd LabelHub   # git clone 后的仓库根目录
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
| 线上任务被误下线/数据污染 | 走本文「Phase 0 · 重置」的 `down -v` 重置(幂等 seed 清不掉已有 draft/认领);视频里不要现场修库 |
