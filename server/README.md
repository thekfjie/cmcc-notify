# CMCC Notify Standalone Server

[简体中文](README.md) | [English](README.en.md)

Standalone Server 是独立部署的 CMCC 通知网关，适合监控系统、NAS、脚本、
CI/CD、Agent 和业务服务使用。它提供管理 WebUI、REST API、应用级 Token、
多个 CMCC 通道、通知组和 Docker 部署。

## 获取 Channel API Key

1. [开启手机新消息底层开关](https://mp.weixin.qq.com/s/sWdK7mbKnLOdZmXXOMjAYA)：这是手机系统的新消息基础能力开关；
2. [开启新消息业务](https://rcs.10086.cn/i/#/?RPwXWk9k0yk)：扫码或打开页面，为实际接收通知的中国移动号码开通业务；
3. 进入“新消息ClawBot”应用号后，点击下方菜单栏“绑定/解绑-立即授权”按钮，进入认证流程。完成后即可获得专属 API Key。

在 WebUI 添加或编辑通道时，可直接粘贴纯 `ak_...`，也可以粘贴 ClawBot
返回的整段授权短信。浏览器会立即提取 Key，服务端保存前还会再次规范化；整段
短信不会进入状态文件或日志。配置文件、环境变量和 Secret 文件入口也支持相同
提取行为。

## 数据模型

- **CMCC 通道**：一份命名的 Channel API Key 配置，可填写备注。消息默认发送给
  该 Key 绑定的用户。
- **通知组**：多个 CMCC 通道的集合。发送到通知组时，服务会对每个通道分别
  提交，并返回逐通道结果。
- **通知应用**：业务系统的独立调用身份，拥有自己的 Token、固定通道或通知组，
  以及每分钟请求上限。

## 快速部署

```bash
cd deployments/standalone
cp config.example.yaml config.yaml
mkdir -p secrets data
printf '%s' '<ADMIN_TOKEN>' > secrets/auth_token
printf '%s' '<CMCC_CHANNEL_API_KEY>' > secrets/cmcc_api_key
chmod 600 secrets/*
docker compose up -d --build
```

默认只监听宿主机 `127.0.0.1:8080`。如需公开访问，建议通过 HTTPS 反向代理
发布，并继续限制管理接口的访问范围。

打开 `http://127.0.0.1:8080/`，使用 `auth_token` 对应的管理令牌登录。

## 配置

```yaml
listen: ":8080"
auth_token_file: /run/secrets/auth_token
state_file: /var/lib/cmcc-notify/state.yaml

accounts:
  - name: primary
    note: "运维值班手机"
    enabled: true
    api_key_file: /run/secrets/cmcc_api_key

groups:
  - name: on-call
    channels:
      - primary

applications: []
```

`accounts` 是配置文件和 API 中保留的兼容字段名，WebUI 中统一显示为“CMCC
通道”。启用 `state_file` 后，可以直接在 WebUI 新增、重命名、编辑和删除通道、
通知组及通知应用。完整 API Key 不会回传给浏览器。

支持以下 Secret 来源：

- 管理令牌：`auth_token`、`auth_token_env` 或 `auth_token_file`；
- 通道密钥：`api_key`、`api_key_env` 或 `api_key_file`；
- 应用 Token：推荐从 WebUI 创建，服务端只保存 SHA-256 哈希。

## WebUI

左侧导航包括：

- 概览：服务状态、版本、运行时间与连接通道数；
- 发送消息：测试文字、多媒体、单通道和通知组发送；
- 账户状态：管理 CMCC 通道、通知组和通知应用；
- API 文档：开通入口、使用指南和可复制请求示例。

WebUI 主要用于管理和联通测试。生产系统应使用通知应用 Token 调用
`POST /v1/notify`。

## 通知应用

在“账户状态 → 通知应用”创建应用，并固定绑定一个 CMCC 通道或通知组。完整
Token 只显示一次，格式为 `cn_app_...`。请立即保存到调用系统的 Secret；泄露
时可在 WebUI 轮换，旧 Token 会立即失效。

```bash
curl -X POST http://localhost:8080/v1/notify \
  -H "Authorization: Bearer <APPLICATION_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"title":"服务告警","message":"磁盘使用率超过 90%"}'
```

请求正文支持：

```json
{
  "title": "可选标题",
  "message": "消息正文"
}
```

也可以只传 `text`。`message` 与 `text` 不能同时使用。应用的发送通道由服务端
配置固定，调用方不能在请求中改写目标。

## 管理与测试 API

以下接口使用管理令牌：

```text
GET    /healthz
GET    /v1/status
GET    /v1/settings
POST   /v1/accounts
PUT    /v1/accounts/{name}
DELETE /v1/accounts/{name}
POST   /v1/groups
PUT    /v1/groups/{name}
DELETE /v1/groups/{name}
POST   /v1/applications
PUT    /v1/applications/{name}
DELETE /v1/applications/{name}
POST   /v1/applications/{name}/rotate-token
POST   /v1/send
POST   /v1/send/media
```

发送到单个通道：

```bash
curl -X POST http://localhost:8080/v1/send \
  -H "Authorization: Bearer <ADMIN_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"account":"primary","text":"backup completed"}'
```

发送到通知组：

```bash
curl -X POST http://localhost:8080/v1/send \
  -H "Authorization: Bearer <ADMIN_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"group":"on-call","text":"service unavailable"}'
```

创建通知组：

```json
{
  "name": "on-call",
  "channels": ["primary", "backup"]
}
```

## 多媒体

本地上传会先使用目标通道的 Channel API Key 上传文件，再发送 CMCC 媒体消息：

```bash
curl -X POST http://localhost:8080/v1/send/media \
  -H "Authorization: Bearer <ADMIN_TOKEN>" \
  -F "account=primary" \
  -F "type=IMAGE" \
  -F "caption=监控截图" \
  -F "file=@./screenshot.png"
```

`caption` 是可选的随附文字。由于真实终端未稳定展示媒体帧中的正文，CMCC
Notify 会先把 `caption` 作为一条纯文本消息发送，再发送一条不带正文的媒体
消息；未填写 `caption` 时只发送媒体消息。

通知组上传会为每个通道分别执行上传与发送。接口支持 `AUTO`、`IMAGE`、
`AUDIO`、`VIDEO` 和 `FILE`，单文件上限为 200 MiB。也可以在 `/v1/send` 中
提供远程媒体 URL，但建议优先使用本地上传流程。

## 内容展示与返回值

2026 年 9 月 17 日真实终端测试结果：

- 文字按纯文本展示并保留换行；
- URL 可被客户端自动识别；
- Unicode 与 Emoji 可以正常显示；
- Markdown、HTML、表格和代码块不会被格式化渲染；
- 图片和文件已通过先上传、后发送的流程验证。
- CMCC Notify 0.4.0 的真实终端验证确认：媒体随附文字先作为独立纯文本显示，
  随后出现单独的多媒体消息和客户端网页入口，顺序与服务端提交顺序一致。

HTTP `202 Accepted` 表示请求已写入 CMCC 网关；通知组部分通道失败时返回
HTTP `207 Multi-Status`。这些状态不等于终端送达或已读回执。

## 本地构建

```bash
cd server
GOWORK=off go test ./...
GOWORK=off go vet ./...
make web-build
make build VERSION=dev
```

构建容器：

```bash
docker build -f server/Dockerfile -t cmcc-notify/server:dev .
```

## 安全建议

- 使用 Secret 文件或环境变量提供凭据；
- 不要将 Token 放入 URL、源码、镜像层或日志；
- 为不同业务系统创建不同通知应用；
- 为应用设置合理的每分钟请求上限；
- 公网部署使用 HTTPS、反向代理限流和访问控制。

## License

MIT
