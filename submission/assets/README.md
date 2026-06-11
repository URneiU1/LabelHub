# submission/assets

本目录存放评委交付素材。源码仓库内只保存体积适中的文档与截图;演示视频二进制通常由最终提交平台或外部附件承载,不直接提交到 Git。

## 已入库素材

| 文件 | 用途 |
|---|---|
| [`AI-Coding-Process.pdf`](AI-Coding-Process.pdf) | AI Coding 过程记录 PDF |
| [`AI-Coding-Process.md`](AI-Coding-Process.md) | PDF 源文件,便于最终提交前微调 |
截图均按验收要求的 **1280×800 与 1920×1080** 两档分辨率重截(当前 UI):

| 文件 | 用途 |
|---|---|
| [`screenshots/01-owner-tasks-1920.png`](screenshots/01-owner-tasks-1920.png) | Owner 任务管理（统计卡 + 任务表 + 生命周期动作）· 1920×1080 |
| [`screenshots/01-owner-tasks-1280.png`](screenshots/01-owner-tasks-1280.png) | Owner 任务管理 · 1280×800 |
| [`screenshots/02-designer-1920.png`](screenshots/02-designer-1920.png) | Designer 三栏（物料 / 画布 / 属性）· 1920×1080 |
| [`screenshots/02-designer-1280.png`](screenshots/02-designer-1280.png) | Designer 三栏（1280 仍保持三栏）· 1280×800 |
| [`screenshots/03-labeler-answer-1920.png`](screenshots/03-labeler-answer-1920.png) | Labeler 作答页（Tabs + 多物料 + 3s 自动保存）· 1920×1080 |
| [`screenshots/03-labeler-answer-1280.png`](screenshots/03-labeler-answer-1280.png) | Labeler 作答页（Tabs + 多物料 + 3s 自动保存）· 1280×800 |
| [`screenshots/04-reviewer-detail-1920.png`](screenshots/04-reviewer-detail-1920.png) | Reviewer 详情 + AI 预审结论（verdict / 维度分 / 处理日志审计）· 1920×1080 |
| [`screenshots/04-reviewer-detail-1280.png`](screenshots/04-reviewer-detail-1280.png) | Reviewer 详情 + AI 预审结论 · 1280×800 |
| [`screenshots/05-export-config-1920.png`](screenshots/05-export-config-1920.png) | Owner 多格式导出配置（8 格式 json/jsonl/csv/xlsx/md/coco/sft/dpo + DPO 偏好对字段映射）· 1920×1080 |
| [`screenshots/05-export-config-1280.png`](screenshots/05-export-config-1280.png) | Owner 多格式导出配置（8 格式含 SFT/DPO 训练格式 + 字段映射）· 1280×800 |

## 视频交付

演示视频按 [`../DEMO_SCRIPT.md`](../DEMO_SCRIPT.md) 录制:

- 建议输出:1920x1080 / 30fps / H.264 MP4 / 不超过 200MB
- 推荐命名:`demo-5min.mp4` 或 `demo-10min.mp4`
- 视频文件不放入 Git;最终提交时上传到比赛平台或作为外部附件提交

## 架构图

[`../ARCHITECTURE.md`](../ARCHITECTURE.md) 已内置 Mermaid topology、状态机与 outbox sequence。若最终提交平台需要单独图片,可从该文档渲染 PNG/SVG 后附加上传。

## 最终提交前检查

- [x] AI Coding PDF 已生成
- [x] 关键路径截图已入库
- [x] 演示脚本已入库
- [ ] 最终提交前录制并上传演示视频
