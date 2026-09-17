# CMCC Notify Standalone Server

Standalone Server 面向不使用 Gotify 的用户，提供独立 WebUI、应用级通知
API、多个 CMCC 通道、本地号码组和 Docker 部署。WebUI 主要用于管理和联通
测试；业务系统应使用独立应用 Token 调用通知接口。

## 先理解五个概念

| 概念 | 作用 |
| --- | --- |
| CMCC 通道 | 一份命名 Channel 配置，包含 API Key、连接状态、可选默认 `to` 及协议地址。兼容 API 字段仍叫 `account`。 |
| `to` 目标 | 已验证发送帧中的实际路由数据，不是手机号备注。目标权限仍由 CMCC 网关控制。 |
| 本地号码组 | Standalone Server 保存的一组 `to` 字符串，不是中国移动群聊，也不是完整接收人实体。 |
| 本地扇出 | 服务使用同一 CMCC 通道对号码组成员逐个发送 N 个一对一帧，不是 CMCC 原生广播。 |
| 通知应用 | 供监控、脚本或业务系统调用的独立身份，每个应用拥有独立 Token，并绑定通道、固定 `to` 或号码组及请求频率上限。 |

本地号码组不会在中国移动侧创建任何群聊，也不会让组内目标互相看到。服务收到一次号码组请求后，会使用选中的同一通道，依次为组内每个 `to` 发送一条独立消息，并返回逐目标网关写入结果。上传文件只上传一次，随后复用 CMCC 返回的媒体 URL 逐个发送。

完整术语、证据边界和未来接收人模型见 [`docs/concepts.md`](../docs/concepts.md)。

## 能力边界

中国移动官方安装指引为
[`channel-guide.md`](https://5gvas01.cmicmaap.com/aifile/public/file/channel-guide.md)。
官方指引说明中国移动新消息 Channel 支持发送文字和多媒体。CMCC Notify 将底层通信能力适配为两个独立产品：Gotify Plugin 和 Standalone Server；运行和部署均不依赖 OpenClaw。

2026-09-16 已完成真实端到端验证：PNG 图片和 ZIP 文件均通过 CMCC 上传接口取得媒体 URL，再使用包含明确 `to` 收件号码的富媒体帧发送；网关返回 `media_processed`，目标中国移动新消息终端成功收到两种附件。

- 文字和多媒体都必须解析出明确 `to` 目标；单目标发送可使用通道的默认值；
- `to` 与 `group` 不能同时填写；号码组是本地逐目标扇出，不是中国移动侧的群聊、主题或原生群组接口；
- 图片、音频、视频、文件使用同一上传和发送流程，但目前只实际验证了 PNG 图片与 ZIP 文件；
- 页面或 API 返回“已接受”，只表示内容成功写入 CMCC WebSocket；`media_processed` 表示网关处理了媒体，但仍不是送达或已读回执；
- 是否允许向任意未开通、未授权或非中国移动号码投递，公开指引没有明确说明，部署者应控制目标列表和发送频率。

## 1. 开通中国移动新消息

使用需要接收消息的中国移动号码打开下面的页面，并按照页面提示完成开通：

[打开中国移动新消息开通页面](https://rcs.10086.cn/i/#/?RPwXWk9k0yk)

Standalone WebUI 的“API 文档 → 使用指南”中也提供了这个入口和可供手机扫描的二维码。

## 2. 登录 WebUI

启动服务后打开：

```text
http://<server>:8080/
```

输入服务端配置的 `auth_token`、`auth_token_env` 或
`auth_token_file` 对应的管理令牌。令牌只保存在当前浏览器会话中，关闭会话或点击“退出”后会清除。

## 3. 管理 CMCC 通道

进入“账户状态 → CMCC 通道”。

### 添加通道

点击“添加通道”，填写：

- 通道名称：本服务内部使用的唯一名称，例如 `primary`。
- Channel API Key：以 `ak_` 或 `app_` 开头。启用通道时必须填写。
- 默认目标号码（`to`）：可选；用于单目标文字和多媒体发送时的默认值。
- WebSocket 地址：可选；留空使用 SDK 的官方默认地址。
- 上传 API 地址：可选；留空使用 SDK 的官方默认地址。
- 启用通道：启用后会立即连接 CMCC，无需重启服务。

保存后，状态显示“已连接”才表示 WebSocket 认证和连接成功。显示“未连接”时仍可尝试发送，服务会先重新连接。

### 编辑通道

点击通道卡片中的“编辑”。通道名称可以修改；重命名时，所有引用旧名称的通知应用会同步迁移到新名称，不需要逐个修改。API Key 输入框留空表示保留现有密钥；输入新值表示替换。保存后相关连接会自动重建。

### 删除通道

点击“删除”并确认。删除通道不会删除其他通道和本地号码组，但之后不能再选择该通道发送。

完整 API Key 不会通过状态或设置 API 回传给浏览器，只显示摘要。

## 4. 通知应用（生产接入推荐）

“发送消息”页面适合人工验证通道、`to`、号码组和多媒体链路。正式接入监控、
定时任务、NAS、CI/CD 或业务系统时，建议为每个调用方创建独立通知应用，
不要向业务系统分发管理令牌或 CMCC API Key。

进入“账户状态 → 通知应用 → 新建应用”，配置：

- 应用名称，例如 `monitoring`、`nas`、`billing`；
- 一个 CMCC 发送通道；
- 一个固定 `to` 目标或本地号码组；
- 每分钟请求上限，默认 60，可设置 1–1000；
- 是否允许调用方临时覆盖发送目标，默认关闭；
- 是否启用应用。

创建后完整 Token 只显示一次，格式为 `cn_app_...`。服务端只保存 Token 的
SHA-256 哈希；请立即将完整 Token 写入调用系统的 Secret。Token 不应放入 URL、
源码、前端页面或日志。发生泄露时，在应用卡片中点击“轮换 Token”，旧 Token
会立即失效；停用或删除应用也会立即阻止后续调用。

业务系统发送通知：

```bash
curl -X POST http://localhost:8080/v1/notify \
  -H "Authorization: Bearer <APPLICATION_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"title":"服务告警","message":"磁盘使用率超过 90%"}'
```

`title` 和 `message` 会以空行连接后发送；也可只传 `message`，或使用 `text`
作为纯文本别名。`message` 与 `text` 不能同时使用。默认情况下请求不能携带
`to` 或 `group`，因此即使应用 Token 泄露，发送范围仍被限制在应用绑定目标。
只有显式开启“允许调用方覆盖目标”的应用，才能在请求中传入二者之一。

超过应用频率上限时返回 HTTP `429 Too Many Requests` 和 `Retry-After`。成功
返回 HTTP `202 Accepted`，含应用名称和 CMCC `message_id`；它仍只表示已写入
网关，不是送达或已读回执。

内置限流是单进程内的一分钟固定窗口，服务重启后重新计数；多副本部署时每个
副本独立计数。公网部署仍建议在反向代理或 API Gateway 增加全局限流与来源控制。

## 5. 发送单目标文字消息

进入“发送消息”，依次选择：

1. “文字消息”；
2. 发送通道；
3. “单个目标”；
4. 填写目标号码（`to`）和消息内容；
5. 点击“提交到 CMCC 网关”。

目标号码可以留空，但所选通道必须已经配置默认 `to`。

REST API 示例：

```bash
curl -X POST http://localhost:8080/v1/send \
  -H "Authorization: Bearer <CMCC_NOTIFY_AUTH_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"account":"primary","to":"13800138000","text":"hello"}'
```

## 6. 本地号码组与扇出

### 创建号码组

进入“账户状态 → 本地号码组 → 新建号码组”。

- 号码组名称必须唯一，例如 `family`、`team`。
- 每行填写一个 `to` 目标。
- 也可以使用英文/中文逗号或分号分隔。
- 空白项会忽略，重复目标会自动去重。
- 号码组名称创建后不能直接重命名；需要新建号码组并删除旧号码组。

例如在输入框中填写：

```text
13800138000
13900139000
13700137000
```

### 编辑或删除号码组

在号码组卡片中可以查看完整 `to` 列表、修改目标或删除号码组。删除只会移除本地列表，不会影响 CMCC 通道，也不会在中国移动侧执行任何群聊操作。

### 执行本地扇出

进入“发送消息”，依次选择：

1. 选择“文字消息”或“多媒体”；
2. 发送通道；
3. “本地号码组”；
4. 选择号码组；
5. 填写文字，或选择文件/远程 URL 后提交。

文字消息会逐个调用 `SendText`；多媒体会先上传一次，再逐个调用带不同 `to` 的 `SendMedia`。所以一个号码组有 10 个目标，就会产生 10 次独立网关写入。某一个目标失败不会阻止后续目标继续处理。这是 CMCC Notify 的本地扇出，不是 CMCC 原生群发 API。

REST API 示例：

```bash
curl -X POST http://localhost:8080/v1/send \
  -H "Authorization: Bearer <CMCC_NOTIFY_AUTH_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"account":"primary","group":"family","text":"hello family"}'
```

本地扇出响应示例：

```json
{
  "mode": "local_fanout",
  "native_broadcast": false,
  "group": "family",
  "total": 2,
  "accepted_count": 2,
  "failed_count": 0,
  "results": [
    {
      "to": "13800138000",
      "message_id": "msg_...",
      "accepted": true,
      "acknowledged": false
    },
    {
      "to": "13900139000",
      "message_id": "msg_...",
      "accepted": true,
      "acknowledged": false
    }
  ]
}
```

全部目标写入成功时返回 HTTP `202 Accepted`；部分成功、部分失败时返回 `207 Multi-Status`；全部失败时返回 `502 Bad Gateway`。`accepted_count` 表示成功写入网关连接的数量，不是最终送达数量。调用方应检查 HTTP 状态、`failed_count` 和 `results[]`，不要只判断请求是否拿到 JSON。

### 本地扇出的当前限制

- 当前发送是请求内依次处理，不是定时任务或后台队列。
- `accepted_count` 只能反映 WebSocket 写入是否成功，不能据此判断某个 `to` 是否被网关授权或最终投递。
- 目前只验证了已开通目标的实际收取；同一 Channel API Key 对多个不同号码的授权范围仍由 CMCC 决定，号码组不扩大 Key 本身的权限。
- 没有可靠的送达/已读回执，不能把 `accepted: true` 当成对方已经收到。

## 7. 发送多媒体

多媒体使用与文字相同的明确 `to` 路由。2026-09-16 已在真实中国移动新消息终端验证 PNG 图片与 ZIP 文件可以按 `to` 手机号送达。其他图片、音频、视频和文件格式仍取决于 CMCC 网关和接收终端支持情况。

### 本地上传

进入“发送消息 → 多媒体 → 本地上传”：

1. 选择发送通道；
2. 选择“单个目标”并填写 `to`，或选择一个本地号码组；
3. 媒体类型保持“自动识别”，或手动选择图片、音频、视频、文件；
4. 选择最大 200 MiB 的本地文件；
5. 可填写附言并提交。

服务会先将文件上传到 CMCC `/upload` 接口，再把返回的媒体 URL、`to` 目标、文件名、大小、MIME 类型和媒体类型写入 WebSocket 消息。号码组发送复用同一个媒体 URL。

```bash
curl -X POST http://localhost:8080/v1/send/media \
  -H "Authorization: Bearer <CMCC_NOTIFY_AUTH_TOKEN>" \
  -F "account=primary" \
  -F "to=13800138000" \
  -F "type=AUTO" \
  -F "caption=photo" \
  -F "file=@./photo.jpg"
```

`type` 可使用 `AUTO`、`IMAGE`、`AUDIO`、`VIDEO` 或 `FILE`。

### 远程 URL（高级模式）

远程文件必须使用 `http://` 或 `https://`，并且 CMCC 网关能够直接访问。

```json
{
  "account": "primary",
  "to": "13800138000",
  "media": {
    "type": "IMAGE",
    "url": "https://example.invalid/image.jpg",
    "caption": "photo"
  }
}
```

## 8. 配置与持久化

```yaml
listen: ":8080"
auth_token_file: /run/secrets/auth_token
state_file: /var/lib/cmcc-notify/state.yaml

accounts:
  - name: primary
    enabled: true
    api_key_file: /run/secrets/cmcc_api_key
    default_to: "13800138000"

groups:
  - name: family
    recipients:
      - "13800138000"
      - "13900139000"

applications: []
```

`state_file` 是 WebUI 通道、号码组和通知应用管理的必要配置：

- 未配置时，WebUI 管理功能为只读。
- 所在目录必须允许服务进程写入。
- WebUI 变更会以原子替换方式写入，文件权限为 `0600`。
- 通过 `api_key_file` 或 `api_key_env` 提供的密钥仍保存为引用，不会复制明文到状态文件。
- 号码组中的 `to` 目标会以明文保存在状态文件中，部署者应限制该目录的主机访问权限并做好备份。
- WebUI 创建的应用只保存 Token 哈希和脱敏提示，不保存可直接调用的完整 Token。

环境变量运行示例：

```bash
export CMCC_API_KEY='ak_...'
export CMCC_NOTIFY_AUTH_TOKEN='generate-a-long-random-token'
go run ./cmd/cmcc-notify -config ./config.example.yaml
```

Docker 可以使用只读 secret 文件，并把 `/var/lib/cmcc-notify` 挂载为持久化目录。参见 [`deployments/standalone/`](../deployments/standalone/)。

## 9. REST API 一览

管理、配置、联通测试和多媒体接口使用管理令牌：

```http
Authorization: Bearer <CMCC_NOTIFY_AUTH_TOKEN>
```

`POST /v1/notify` 使用对应应用自己的 `APPLICATION_TOKEN`。两类 Token 都只能放
在 `Authorization` 请求头中；不提供 query-string Token 方式，避免被访问日志、
浏览器历史或反向代理记录。

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| `GET` | `/healthz` | 无认证健康检查。 |
| `GET` | `/v1/status` | 服务版本、运行时间和通道连接状态。 |
| `GET` | `/v1/settings` | 获取可管理的通道摘要与完整号码组目标。 |
| `POST` | `/v1/accounts` | 添加 CMCC 通道；路径和字段名为兼容性保留。 |
| `PUT` | `/v1/accounts/{name}` | 编辑或重命名通道；空 `api_key` 保留原密钥，重命名会同步通知应用引用。 |
| `DELETE` | `/v1/accounts/{name}` | 删除通道。 |
| `POST` | `/v1/groups` | 创建本地号码组。 |
| `PUT` | `/v1/groups/{name}` | 更新号码组的 `to` 目标。 |
| `DELETE` | `/v1/groups/{name}` | 删除本地号码组。 |
| `POST` | `/v1/applications` | 创建通知应用并一次性返回完整应用 Token。 |
| `PUT` | `/v1/applications/{name}` | 编辑应用路由、限流与启用状态。 |
| `POST` | `/v1/applications/{name}/rotate-token` | 轮换 Token；旧 Token 立即失效。 |
| `DELETE` | `/v1/applications/{name}` | 删除应用并吊销 Token。 |
| `POST` | `/v1/notify` | 使用应用 Token 发送生产文字通知。 |
| `POST` | `/v1/send` | 发送单目标或本地扇出文字/远程 URL 多媒体。 |
| `POST` | `/v1/send/media` | 上传本地文件并发送给单个 `to` 或本地号码组。 |

创建通道（兼容 API 字段仍使用 `account`）：

```json
{
  "name": "backup",
  "api_key": "ak_...",
  "enabled": true,
  "default_to": "13800138000",
  "server_url": "",
  "upload_url": ""
}
```

创建本地号码组（`recipients` 当前保存 `to` 字符串）：

```json
{
  "name": "family",
  "recipients": ["13800138000", "13900139000"]
}
```

## 10. 返回结果如何理解

HTTP `202 Accepted` 或 JSON 中的 `accepted: true` 只表示内容已经写入 CMCC 网关连接。当前已知协议没有可靠的送达或已读回执，因此：

- 不代表对方终端一定收到；
- 不代表对方已经阅读；
- 不代表终端一定支持展示对应多媒体类型；
- `acknowledged: false` 通常表示协议没有提供远端确认，而不是必然发送失败。

## 11. 构建与测试

服务器模块可脱离根目录 `go.work` 独立检查：

```bash
GOWORK=off go test ./...
make build
```

修改 `web/` 后构建嵌入式 WebUI：

```bash
make web-install
make web-check
make web-build
```

从仓库根目录构建容器：

```bash
docker build -f server/Dockerfile -t cmcc-notify/server:local .
```

匹配 `server/vX.Y.Z` 的标签会发布 Linux AMD64/ARM64 二进制和多架构容器镜像。
