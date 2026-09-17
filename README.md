# CMCC Notify

[简体中文](README.md) | [English](README.en.md)

CMCC Notify 是一个面向中国移动新消息能力的开源通知网关。本仓库以一个
monorepo 同时维护两个产品，并共享同一套 Go SDK：

| 使用场景 | 产品 | 文档 |
| --- | --- | --- |
| 已经部署 Gotify | Gotify Plugin：将 Gotify 消息转发到 CMCC | [gotify-plugin/](gotify-plugin/) |
| 需要独立通知服务 | Standalone Server：提供 WebUI、REST API、应用 Token 与 Docker 部署 | [server/](server/) |

## 主要能力

- 多个 CMCC 通道，每个通道保存一份 Channel API Key 与可选备注；
- 文字消息、图片和文件上传发送；
- 通知组：一次请求向组内多个 CMCC 通道分别发送；
- 通知应用：为监控、脚本、NAS、CI/CD 或 Agent 分配独立 Token；
- WebSocket 认证、心跳、自动重连、上传和结构化错误处理；
- AMD64/ARM64 二进制、Docker 镜像与独立产品 Release。

一个 CMCC 通道对应一份 Channel API Key，消息默认投递给该 Key 绑定的用户。
通知组保存的是多个通道名称，组发送
会产生多次独立提交，并返回逐通道结果。

## 获取 Channel API Key

1. [开启手机新消息底层开关](https://mp.weixin.qq.com/s/sWdK7mbKnLOdZmXXOMjAYA)：用于确认手机系统已启用新消息基础能力；
2. [开启新消息业务](https://rcs.10086.cn/i/#/?RPwXWk9k0yk)：使用需要接收通知的中国移动号码扫码或打开页面完成业务开通；
3. 进入“新消息ClawBot”应用号，点击下方菜单栏“绑定/解绑-立即授权”，完成认证后获取专属 API Key。

Standalone WebUI 的 Channel API Key 输入框既可以粘贴纯 `ak_...`，也可以直接
粘贴 ClawBot 返回的整段授权短信；前后端都会只提取并保存其中的 API Key。

## 仓库结构

```text
cmcc-notify/
├── cmcc/                  共享 CMCC Go SDK（独立 Go Module）
├── gotify-plugin/         Gotify Plugin（独立 Go Module）
├── server/                Standalone Server（独立 Go Module）
├── integrations/          可选集成示例
├── deployments/           部署示例
├── docs/                  架构与协议说明
├── .github/workflows/     独立测试、构建与发布流程
└── go.work                本地联合开发配置
```

三个模块可以在 `GOWORK=off` 下独立构建。Standalone Server 的 Web、存储等依赖
不会进入 Gotify Plugin 的依赖图。

## 快速开始

- Standalone Server：[中文文档](server/README.md) · [English](server/README.en.md)
- Gotify Plugin：[中文文档](gotify-plugin/README.md) · [English](gotify-plugin/README.en.md)
- Go SDK：[cmcc/README.md](cmcc/README.md)
- 系统架构：[docs/architecture.md](docs/architecture.md)
- 协议说明：[docs/cmcc-protocol.md](docs/cmcc-protocol.md)

## 内容展示说明

2026 年 9 月 17 日真实终端测试结果：文字按纯文本展示，保留换行，客户端会
自动识别 URL，并支持 Unicode 与 Emoji。Markdown、HTML、表格和代码块可以作为
普通字符串发送，但不会被格式化渲染。图片和文件已验证可通过“先上传、后发送”
的流程投递。

## 开发

```bash
make test
make check
make build-server
make build-plugin
```

也可以分别验证三个模块：

```bash
cd cmcc && GOWORK=off go test ./...
cd ../gotify-plugin && GOWORK=off go test ./...
cd ../server && GOWORK=off go test ./...
```

## 版本与发布

- `cmcc/vX.Y.Z`：共享 Go SDK；
- `gotify-plugin/vX.Y.Z`：Gotify 插件产物；
- `server/vX.Y.Z`：Standalone 二进制与多架构容器镜像。

Gotify Go Plugin 产物与目标 Gotify 版本、Go 版本和架构相关，不能假定跨版本兼容。

## 安全

不要将 CMCC API Key、Gotify Client Token、Standalone 管理令牌或应用 Token
提交到源码、镜像或日志。详细说明见 [SECURITY.md](SECURITY.md)。

## License

MIT
