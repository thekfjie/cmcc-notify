# 核心概念与能力边界

本文定义 CMCC Notify 使用的项目术语。界面、README、配置和 API 文档都应遵循这里的含义，避免把中国移动基础设施、Channel 凭据、目标号码和 OpenClaw 实例混为一谈。

## 一句话定义

> CMCC Notify 是独立的中国移动新消息通知网关，将已经验证的 CMCC Channel 能力封装为 Gotify 插件和 Standalone REST 服务，供监控系统、脚本、Agent 及其他程序提交通知。

```text
外部系统 / Gotify / OpenClaw / 脚本
                 │
          REST API / Gotify stream
                 ▼
           CMCC Notify
                 │
        CMCC 通道凭据 + to 目标
                 ▼
           CMCC Gateway
                 ▼
        中国移动新消息 / RCS
                 ▼
              用户终端
```

## 术语表

| 名称 | 在本项目中的准确含义 |
| --- | --- |
| 中国移动新消息 | 最终承载通知的移动消息能力。公开安装指引声明支持文字和多媒体。 |
| CMCC Channel 入口 | 中国移动分发的 Channel 接入方式。公开安装包面向 OpenClaw，但 CMCC Notify 只复用经过审计和验证的协议能力，不安装或依赖 OpenClaw。 |
| Channel API Key | `ak_...` 或 `app_...` 形式的 Channel 认证凭据。它不是手机号，也不是 CMCC Notify 的管理令牌。 |
| CMCC 通道 | CMCC Notify 中保存的一份命名 Channel 配置，主要包含 API Key、连接状态、协议地址和可选默认目标号码。代码和兼容 API 中仍使用 `account` 字段。 |
| `to` 目标 | 当前已验证发送帧中的实际路由目标。项目实测文字、图片和文件均使用该字段。它不是单纯备注。目标范围和授权规则由 CMCC 网关控制，不能把它宣传为可向任意号码开放发送的短信接口。 |
| 本地号码组 | CMCC Notify 保存的一组 `to` 目标。配置字段为了兼容仍叫 `groups[].recipients`，但其中存储的是目标号码字符串，不是完整“接收人”实体。 |
| 本地扇出 | 选择一个本地号码组后，CMCC Notify 使用同一个 CMCC 通道逐个发送 N 个一对一帧。它不是 CMCC 原生群聊、广播或 `groupId`。 |
| 通知应用 | Standalone 中面向外部系统的调用身份。每个应用拥有独立 Token，并绑定固定通道和固定号码或号码组。 |
| Gotify / OpenClaw / Codex / 监控系统 | 通知产生方或上游系统，不是 CMCC 通道本身。 |
| WebUI | CMCC Notify 的管理和联通测试控制台；真正的发送、扇出、上传和错误处理都在后端执行。 |

## 群发的真实工作方式

CMCC 的公开安装指引没有描述 `recipients[]`、`groupId` 或广播接口。CMCC Notify 当前也没有使用这些未经验证的能力。

```text
本地号码组：服务器管理员
├── 138****0001
├── 138****0002
└── 138****0003

选择通道 primary 并发送一次
        │
        ├── send(to=138****0001)
        ├── send(to=138****0002)
        └── send(to=138****0003)
```

因此一次号码组请求会产生 N 次网关写入。响应中的：

- `mode: "local_fanout"` 明确表示本地扇出；
- `native_broadcast: false` 明确表示没有调用 CMCC 原生广播；
- `accepted_count` 是成功写入网关连接的次数；
- `failed_count` 是写入时返回错误的次数；
- `results[]` 给出每个 `to` 的独立结果；
- 全部写入成功返回 HTTP `202`，部分失败返回 `207`，全部失败返回 `502`。

这些结果都不是终端送达或已读回执，也不能证明某个 `to` 已获得网关授权。号码组只改变 CMCC Notify 的调用次数，不会扩大所选 Channel API Key 的权限。

## 已确认、未确认与未来模型

### 已确认

- WebSocket API Key 鉴权和 `auth_ok`；
- 带 `to` 的文字发送；
- HTTP 上传后使用媒体 URL 发送；
- 带 `to` 的图片和文件发送；
- PNG 图片和 ZIP 文件已在真实终端送达；
- 本地号码组可以通过 N 次一对一帧完成扇出。

### 尚未确认

- CMCC 原生广播、群聊或 `groupId` API；
- `topic` 在当前 Channel 协议中的可用含义；
- 任意目标号码都可发送的权限范围；
- 可靠的终端送达和已读回执；
- 所有音频、视频格式在不同终端上的展示能力。

### 尚未实现的“接收人”模型

当前版本没有独立的 `Recipient` 通讯录实体，也没有“一个接收人绑定 CMCC、Email、Bark 等多个渠道”的跨渠道路由。现在的数据关系是：

```text
通知应用 ──绑定──> CMCC 通道
     │
     └──────────> 固定 to 或本地号码组
```

未来若增加通用接收人模型，应作为新的数据层和迁移方案实现：

```text
Recipient ──> 多个通知渠道
Group ──────> Recipient IDs
Notify Engine ──> 按策略选择渠道并处理重试
```

在该模型真正实现前，界面和文档不应把号码组描述成完整接收人目录，也不应声称每个组成员拥有独立 Channel API Key。
