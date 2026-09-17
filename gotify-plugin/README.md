# CMCC Notify Gotify Plugin

[简体中文](README.md) | [English](README.en.md)

这是一个独立的 Gotify v1 Go Plugin 项目，用于把 Gotify 消息转发到中国移动
新消息。插件使用 Gotify 自带的配置编辑器和插件详情页，不嵌入 Standalone
Server 的 WebUI。

## 构建与测试

```bash
GOWORK=off go test ./...
make check
make build GOTIFY_VERSION=v3.1.1
```

Go Plugin 产物必须与目标 Gotify 的版本、Go 工具链和 CPU 架构匹配。当前构建
目标为 Gotify 3.1.1。

## 配置

在 Gotify 插件页面保存以下 YAML：

```yaml
enabled: true
gotify_url: http://127.0.0.1:80
client_token: C_replace_with_a_dedicated_gotify_client_token
include_title: true
title_separator: "\n\n"

accounts:
  - name: primary
    note: "运维值班手机"
    api_key: ak_replace_with_cmcc_key
    enabled: true

routes:
  - applications: [1, 2]
    accounts: [primary]
    minimum_priority: 0
```

- `client_token` 必须是专用的 Gotify Client Token。插件通过 `/stream` 接收该
  用户可见的消息，不要填写 Gotify Application Token。
- `accounts` 中每一项是一条 CMCC 通道，包含 Channel API Key、名称、可选备注
  和启用状态，消息默认发送给该 Key 绑定用户。
- `routes[].applications` 是 Gotify Application ID 列表；空列表表示全部应用。
- `routes[].accounts` 是消息需要转发到的 CMCC 通道列表。
- `minimum_priority` 只用于 Gotify 插件本地筛选。CMCC 消息本身不携带 Gotify
  Priority 样式或优先级展示。

没有配置 `routes` 时，每条 Gotify 消息会转发到所有已启用 CMCC 通道。一个
路由选择多个通道时，插件会为每个通道分别发送。

## Gotify 插件详情页

插件实现了 Gotify 的 `Configurer`、`Displayer`、`Storager` 和 `Webhooker`
能力。Gotify 统一插件页面会显示：

- 插件与 Gotify 消息流状态；
- CMCC 通道连接状态和脱敏后的 API Key；
- 路由规则、成功/失败计数与最近错误；
- 插件直连接口路径。

通道重命名后，应在同一次配置修改中同步更新 `routes[].accounts`。

## 插件直连接口

Gotify 会把自定义接口挂载在插件 Token 路径下：

```text
POST /plugin/<id>/custom/<plugin-token>/send
GET  /plugin/<id>/custom/<plugin-token>/status
```

发送文字：

```json
{
  "account": "primary",
  "text": "backup completed"
}
```

发送远程媒体：

```json
{
  "account": "primary",
  "media": {
    "type": "IMAGE",
    "url": "https://example.com/screenshot.png",
    "caption": "monitoring screenshot"
  }
}
```

`caption` 不会写入媒体帧正文。插件会先发送一条纯文本，再发送不带正文的
媒体消息；未填写 `caption` 时会使用 Gotify 消息正文作为随附文字。这样可避
免终端收到媒体却不显示说明文字。

远程媒体 URL 必须能被 CMCC 网关访问。需要上传本地文件时，建议使用
Standalone Server 的 `/v1/send/media`。

## 内容展示

2026 年 9 月 17 日真实终端测试结果：文字按纯文本展示，保留换行，自动识别
URL，并支持 Unicode 与 Emoji。Markdown、HTML、表格和代码块不会被格式化
渲染。插件可继续转发这些字符串，但不会获得 Gotify 中的 Markdown 展示效果。
CMCC Notify 0.4.0 的真实终端验证确认，媒体随附文字会先作为独立纯文本显示，
随后出现单独的多媒体消息和客户端网页入口。

HTTP `202 Accepted` 或 SDK 返回 `accepted: true` 只表示请求已写入 CMCC 网关，
不代表终端送达或已读。

## 安装

把与 Gotify 版本和架构匹配的 `.so` 文件放入 Gotify 插件目录，然后重启 Gotify。
生产升级前请先备份 Gotify 数据库与插件配置。

## 社区收录

提交 `gotify/contrib` 时可以直接引用：

```text
https://github.com/thekfjie/cmcc-notify/tree/main/gotify-plugin
```

## License

MIT
