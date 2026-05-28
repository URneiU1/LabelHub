# submission/assets — 视频 / 截图 / PDF 占位

本目录存放评委提交所需的二进制素材。文件本身在最终提交前由作者补齐。

## 待补文件清单

### 1. `demo.mp4`(或 `demo-5min.mp4` / `demo-10min.mp4`)

- 录制要求:1920×1080 / 30fps / H.264 MP4 / 不超过 200MB
- 内容脚本:[`../DEMO_SCRIPT.md`](../DEMO_SCRIPT.md)
- 录制工具:OBS / Screen Studio / macOS 系统录屏
- 录制前 checklist 见 `DEMO_SCRIPT.md` 顶部

### 2. `architecture.png`(或 `.svg`)

- 用途:嵌入 PPT、文档封面、社交分享
- 渲染方式:`docs/ARCHITECTURE.md` 已含 3 个 Mermaid 块(topology / 状态机 / outbox sequence)
  ```bash
  # 安装 mermaid-cli
  npm install -g @mermaid-js/mermaid-cli
  # 渲染(单文件 = 单图,多块需逐一)
  mmdc -i ../ARCHITECTURE.md -o architecture.png -w 1920
  ```
- 备选:VS Code 装 Mermaid Preview 插件 → 截图

### 3. `AI-Coding-Process.pdf`(8-15 页)

- 用途:展示 AI Coding 过程(评分维度 25% 工程质量的"附加")
- 原料:`../../docs/CHANGELOG.md`(50 条 conventional commits 按 S0-S7 分章)
- 推荐结构:
  - 封面 + 摘要(1 页)
  - Sprint 0-1 启动 & 状态机奠基(2 页)
  - Sprint 2 模板 Designer 物料(2-3 页,含 9 + 2 物料截图)
  - Sprint 3 AI 预审产品化(3-4 页,含 Function Calling / dry-run / 熔断)
  - Sprint 4 异步导出 + Stats Board(1-2 页)
  - Sprint 5 工程质量(testcontainers / 集成测试 / strict mode)(1-2 页)
  - Sprint 6 Editorial UI + a11y(1 页)
  - Sprint 7 收尾 + 4 路并行 Code Review(1 页)
  - 关键 AI Coding 决策点(2 页,Claude 怎么参与 / 何时驳回 / 何时迭代)
- 写作工具:Notion / Typora / Pages / Word 导 PDF

### 4. `screenshots/`(子目录)

至少四张:

| 文件名 | 内容 |
|---|---|
| `01-owner-designer.png` | Owner Designer 三栏布局(物料 / 画布 / 属性)+ 嵌套 Tabs/Group 预览 |
| `02-labeler-plaza.png` | Labeler 答题页(ShowItem 多媒体 + Radio/Tags/RichText/LLMTrigger 同框) |
| `03-reviewer-detail.png` | Reviewer 详情(左 Labeler 答案 + 右 AI verdict + 三维度柱) |
| `04-export-history.png` | 导出 history(queued → running → succeeded + 下载) |

加分:
- `05-stats-board.png` — Stats Board 的进度/通过率/AI vs 人工 chart
- `06-golden-sample-dry-run.png` — Golden Sample dry-run history/trend
- `07-mobile-or-1280.png` — 1280 响应式(属性面板下移)

### 5.(可选)`pitch-deck.pdf`

若提交需附 PPT,放这里。否则跳过。

---

## 提交前最终 checklist

- [ ] `demo.mp4` 存在且 ≤200MB
- [ ] `architecture.png` 存在
- [ ] `AI-Coding-Process.pdf` 存在且 8-15 页
- [ ] `screenshots/` 至少 4 张
- [ ] 本 README 删除 "待补" 字样

提交后,本目录的 `.gitignore` 应排除以下大文件(避免炸仓):
```
*.mp4
*.mov
*.zip
```

(若仓库已有 `.gitignore` 规则覆盖,无需重复。)
