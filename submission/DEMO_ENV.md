# LabelHub · 可访问演示环境说明（任意云平台部署）

> 交付清单第 5 项 · 面向评委的「在线 demo 入口」+「任意云平台一键复现」说明。
> 技术 SOP 细节见 [`DEPLOY.md`](DEPLOY.md);本机 5 分钟评估见 [`README.md`](README.md)。

---

## 一、在线访问（评委首选）

| 项 | 值 |
|---|---|
| 访问地址 | **http://43.155.210.70** （腾讯云轻量·首尔节点,免备案) |
| asynqmon 队列后台 | `http://43.155.210.70/asynqmon/`（basic auth,账号 `admin`,密码见部署交付,不入库) |
| 健康检查 | `http://43.155.210.70/health` → 返回 `ok` |

> 当前为公网 IP 直连(HTTP)。如需 HTTPS,挂任意域名解析到该 IP 并把 `CADDY_SITE_ADDRESS` 改成域名即可,Caddy 自动签发证书(首尔节点海外免备案)。

### 评估账号（密码均为 `123456`）

| 用户名 | 角色 | 评估场景 |
|---|---|---|
| `owner1` | Owner | 任务配置 / 模板 Designer / AI Prompt / Golden Sample / Stats Board / 多格式导出 |
| `labeler1` | Labeler | 任务广场领题 / 9+2 物料答题 / 草稿自动保存 / 提交 |
| `reviewer1` | Reviewer | 审核队列 / AI verdict + 维度评分 / 通过-打回-修订 / 批量审核 |
| `admin1` | Admin | 全局管理 |

> 演示数据由 seed 自动写入 2 个官方任务(`qa_quality` / `preference_compare`)及 9 个 demo 账号(owner1/2、labeler1-3、reviewer1/2、admin1、system_ai)。5 分钟跑通三角色完整链路的脚本见 [`DEMO_SCRIPT.md`](DEMO_SCRIPT.md)。

---

## 二、为什么是"任意云平台"

LabelHub 整套打包成 **7 个 Docker 服务**(`mysql` / `redis` / `api` / `worker` / `web` / `asynqmon` / `caddy`),由仓库内 `deploy/docker-compose.prod.yml` 编排。它**不依赖任何云厂商专属能力**(无托管数据库、无 Serverless、无对象存储绑定),所以同一份命令在以下平台**完全一致**:

- 阿里云 ECS / 腾讯云 CVM / 华为云 ECS
- AWS EC2 / GCP Compute Engine / Azure VM
- DigitalOcean / Vultr / Hetzner / 任意 VPS

唯一要求:一台装了 Docker 的 **Linux 主机(2 核 4G 起,Ubuntu 22.04)**。

> 说明:本项目是有状态后端(Go 常驻 API + Go worker + MySQL + Redis),**Vercel/Netlify 这类纯前端/Serverless 平台无法承载**,因此统一走「云服务器 + Docker Compose」路径。

---

## 三、一键部署(从空白主机到可访问,约 10 分钟)

```bash
# 1. 装 Docker(任意云的 Ubuntu 主机通用)
curl -fsSL https://get.docker.com | sh

# 2. 拉代码
git clone https://github.com/URneiU1/<repo>.git labelhub && cd labelhub

# 3. 生成生产 env
cp deploy/.env.example deploy/.env
```

编辑 `deploy/.env`,**必填项**用下面命令生成随机密钥后填入:

```bash
openssl rand -hex 32   # → JWT_SECRET
openssl rand -hex 32   # → REDIS_PASSWORD
openssl rand -hex 32   # → EXPORT_DOWNLOAD_SECRET
openssl rand -hex 16   # → MYSQL_PASSWORD / MYSQL_ROOT_PASSWORD(各跑一次)

# asynqmon 后台登录密码哈希:
docker run --rm caddy:2-alpine caddy hash-password --plaintext '你的后台密码'
# → 填入 ASYNQMON_PASSWORD_HASH
```

`deploy/.env` 里跟"访问地址"相关的两项,按是否有域名二选一:

| 场景 | `CADDY_SITE_ADDRESS` | `API_CORS_ORIGINS` | HTTPS |
|---|---|---|---|
| **有域名**(推荐) | `labelhub.你的域名.com` | `https://labelhub.你的域名.com` | Caddy 自动签发,零配置 |
| **仅公网 IP**(临时 demo) | `:80` | `http://你的公网IP` | 无(纯 HTTP) |

AI 预审接真实豆包(火山引擎,OpenAI 兼容);只想跑通架子可保持 `LLM_PROVIDER=mock`:

```bash
LLM_PROVIDER=openai
LLM_BASE_URL=https://ark.cn-beijing.volces.com/api/v3
LLM_API_KEY=<你的豆包 key>
LLM_MODEL=<你的模型 ID>
LLM_ALLOWED_MODELS=<同上>
```

启动:

```bash
# 校验 .env 是否漏填(缺哪个变量会直接报错)
docker compose --env-file deploy/.env -f deploy/docker-compose.prod.yml config

# 构建并后台启动整套(首次约 3-8 分钟编译 Go + 打包前端)
docker compose --env-file deploy/.env -f deploy/docker-compose.prod.yml up -d --build
```

API 启动时**自动跑数据库迁移建表**。seed 是显式、幂等步骤，首次部署或需要恢复演示数据时执行:

```bash
docker compose --env-file deploy/.env -f deploy/docker-compose.prod.yml exec \
  -e SEED_ALLOW_IN_PROD=true api seed
```

---

## 四、云平台必做的两件事(新手最常卡)

1. **开放安全组 80 / 443 端口** —— 云服务器默认只开 22。到「云控制台 → 安全组 → 入方向」放行 80、443(来源 `0.0.0.0/0`),否则外网访问不到。

2. **域名备案(仅国内厂商)** —— 阿里云/腾讯云**大陆节点 + 域名**对外提供网页需 ICP 备案。三种应对:
   - 已备案域名 → 直接用;
   - 不想备案 → 买**香港/新加坡节点**(免备案),域名解析过去即可;
   - 纯临时 demo → 用 `http://公网IP` 直连(`CADDY_SITE_ADDRESS=:80`)。

---

## 五、验证可访问性

```bash
docker compose --env-file deploy/.env -f deploy/docker-compose.prod.yml ps   # 7 服务应为 running/healthy
curl -f https://<你的地址>/health        # → ok
curl -f https://<你的地址>/api/v1/        # → API 正常响应
```

浏览器打开 `https://<你的地址>`,用 `owner1 / 123456` 登录即可开始评估。

### 线上 5 分钟 dry-run 清单

以下步骤基于当前 seed 数据，用于上线后快速确认主链路。线上数据可能因演示操作发生变化，不依赖固定数量断言。

- [ ] 打开部署地址，使用 `owner1 / 123456` 登录，确认 `qa_quality` 任务列表、统计卡片和趋势图可见。
- [ ] 使用 `labeler1 / 123456` 登录，领取一条任务并提交；确认页面进入下一条待标注项。
- [ ] 使用 `reviewer1 / 123456` 登录，确认人工审核队列可打开，`仲裁` tab 可点击且可用键盘聚焦切换。
- [ ] 回到 owner 账号，创建 JSONL 异步导出；确认导出任务完成后可下载。
- [ ] 刷新页面并检查浏览器控制台；确认字体加载、API 请求和页面渲染无 CSP 或 413 异常。

---

## 六、数据与回滚

- 状态全部落在命名 Docker volume(`mysql_data` / `redis_data` / `exports_data` / `caddy_data`),`docker compose down` 不丢数据,`down -v` 才清空。
- 部署/迁移前备份:
  ```bash
  docker compose --env-file deploy/.env -f deploy/docker-compose.prod.yml exec mysql \
    sh -c 'mysqldump -uroot -p"$MYSQL_ROOT_PASSWORD" "$MYSQL_DATABASE"' > labelhub-backup.sql
  ```
- 回滚为镜像级(重新 `up -d --build` 指定旧 commit);若迁移改了数据,配合上面备份恢复。

---

> **当前状态:已部署并验证可访问** —— `http://43.155.210.70`(三角色登录、`/health`、`/api/v1/` 均外网 200)。本节命令对任意云主机一致复现。
