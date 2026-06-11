# LabelHub · 课题要求完成度与超额项对照

> 一页速览:左列是课题书的要求,右列是 LabelHub 的实际交付。**加粗**为超出课题要求的部分。
> 答辩用法:先证明"全部要求达成",再讲超额项与工程取舍。

## 核心功能对照(课题书 §4)

| 课题要求 | LabelHub 交付 |
|---|---|
| 4.1 分发策略「先到先得 / 指派 / 配额抢单」**任选其一** | **三种全部实现**,另加领题租约超时回收、每日提交上限、每人配额 |
| 4.2 物料 ≥10 种(单行/多行/单选/多选/标签/富文本/文件图片/JSON/LLM/ShowItem) | 10/10 全量;**ImageUpload 服务端 MIME 强校验**(上传时内容嗅探 `http.DetectContentType`,提交时白名单复核,伪造 Content-Type 不可绕过) |
| 4.2 进阶:字段联动 / 自定义校验 / 分组与多 Tab | 全有;自定义校验支持 **expr-eval 表达式函数**(必填/长度/正则之外);**模板版本间 schema 破坏性变更检测(`internal/schemadiff` 递归比对、删字段/改控件/加必填/删选项分级 safe/warning/breaking),Owner 保存新版本前「兼容性检查」预知改动是否让历史标注失效** |
| 4.3 草稿自动保存 | 自动保存 + **localStorage 离线兜底**(断网不丢,恢复/丢弃二选一) |
| 4.4 AI Agent:异步队列 / Function Calling / 重试+幂等 | 三条全中,另加 **outbox 事务性投递、熔断器、卡死 sweeper 自愈、DLQ 失败兜底自动转人工、Prompt dry-run 试跑、AI 审核队列可视化(评分维度/tokens/耗时/重试/幂等键全透出)、渲染后 prompt 快照(落库「AI 实际看到的全文」+ sha256 指纹,与发送给 LLM 同源)+ prompt 配置漂移提示(预审所用配置非当前生效版本时告警)** |
| 4.5 人工审核流转:状态机 / 审计 / 批量 / 打回附理由 | 全有,另加 **多级初审/终审、重叠标注交叉仲裁(needs_arbitration)、验收抽检不通过整批打回(作废历史审核强制重走全流程)、Golden Sample 埋题质检** |
| 4.6 导出 ≥4 格式 | **8 格式**(JSON/JSONL/CSV/Excel/Markdown/COCO/**SFT**/**DPO**);**SFT=OpenAI Chat 微调 `messages`、DPO=偏好对 `prompt/chosen/rejected`,均内联质量溯源 metadata,标注产物直达模型微调管线**;异步导出 + 下载历史 + 字段映射;**下载链接 HMAC 签名防越权** |

## 工程质量亮点(课题书 §5 / 验收 25%)

- **并发正确性**:领题/提交走 `SELECT ... FOR UPDATE` + 唯一键兜底,sqlmock 单测覆盖并发竞争路径;AI 评审幂等键为 api/worker 共享单一实现,杜绝两端漂移。
- **测试**:Go 400+ 用例(19 包)、前端 Vitest 覆盖 Designer/Renderer/工作台/审核台关键流程;阈值边界(80/79、60/59)有显式用例。
- **安全**:JWT + RBAC + 任务级数据隔离(reviewer/labeler 仅见被指派任务)、登录限流、上传内容嗅探、LLM 错误信息脱敏(不回显 key/响应体)、asynqmon 管理台 basic-auth;**上线前做过专项安全审查,发现并修复了 AI 审核队列的越权枚举(IDOR)**。
- **可运维**:docker compose 生产栈(Caddy 自动 HTTPS)+ 队列监控 + 在线演示环境;部署前 mysqldump、构建失败旧容器不下线。
- **AI 工程可解释**:每次评审落库原始 Prompt + 维度评分 + verdict 阈值一致性校验;审计时间线以独立 `system_ai` 账户(AI 审核 Agent)身份可追溯。

## 取舍说明(主动讲,别让评委问)

- **移动端适配(可选加分)未做**:平台目标用户是 PC 工作台场景,把工时投在并发正确性与 AI 链路健壮性上。
- **演示批量审核分区为模拟数据**:演示时使用真实数据走 `/reviews/batch`。
