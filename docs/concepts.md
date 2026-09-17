# 核心概念

本文定义 CMCC Notify 在界面、配置和 API 中使用的正式术语。

## CMCC 通道

一条 CMCC 通道是一份命名的 Channel API Key 配置，包含：

- 通道名称；
- 可选备注；
- Channel API Key；
- 启用和连接状态；
- 可选协议地址覆盖。

消息默认投递给该 Key 绑定的用户。配置文件和兼容 API 中保留字段名
`accounts` / `account`，WebUI 统一称为“CMCC 通道”。

## 通知组

通知组保存多个 CMCC 通道名称：

```yaml
groups:
  - name: on-call
    channels:
      - primary
      - backup
```

向通知组发送一次消息时，服务端会对每个通道分别发送，并返回逐通道结果。通知
组适合值班团队、家庭设备或多环境通知，但不会在终端侧创建群聊。

## 通知应用

通知应用用于生产接入。每个应用包含：

- 独立 Application Token；
- 一个固定 CMCC 通道或通知组；
- 启用状态；
- 每分钟请求上限。

业务系统只需调用 `POST /v1/notify`，无需接触管理令牌或 CMCC API Key，也不能
临时改写应用的发送目标。

## 管理令牌与应用 Token

| 凭据 | 用途 |
| --- | --- |
| 管理令牌 | 登录 WebUI，调用管理和联通测试接口 |
| Application Token | 调用单个通知应用的 `/v1/notify` |
| Channel API Key | 连接 CMCC 网关并上传/发送消息 |
| Gotify Client Token | 仅供 Gotify Plugin 读取用户消息流 |

四类凭据不能混用。

## 内容展示

当前真实终端测试表明，文字能力适合描述为：

> Plain Text + 自动 URL 识别 + Unicode/Emoji

Markdown、HTML、表格和代码块仍可作为字符串发送，但不会被格式化渲染。对较长
内容，建议发送简短摘要和普通 URL。

## 状态含义

- HTTP `202`：请求已写入 CMCC 网关；
- HTTP `207`：通知组中部分通道写入失败；
- `accepted: true`：网关接受了该次提交；
- `acknowledged: false`：当前没有可靠的终端送达或已读回执。
