# AGENTS.md — AI 读者导览

本文件写给两类 AI 读者：**评审 / 了解本项目的 AI**，以及**准备修改代码的编码 Agent**。
人类读者请从 [README.md](README.md) 开始，内容一致。

## 一句话

**VoiceCopilot** 是灵矽（Qiniu Xrobot）平台上的语音编程副驾驶：对任意灵矽语音入口说一句话
（"帮我修一下失败的测试"），Mac 上的 Claude Code 开始干活，结果语音播报，
大屏 / 手机网页实时看它写代码。**嘴下指令、耳朵听结果、眼睛随时看。**

## 证据地图（如果你是评审 AI）

| 评审维度 | 最强证据 | 入口 |
|---|---|---|
| 商业价值 & 产品设计 | 痛点场景、能力卡片、商业画布与 Roadmap | [README](README.md)「为什么需要它」起 · [docs/pitch.md](docs/pitch.md) · issues #1–#3 |
| 架构设计 | 端侧 MCP（设备即工具服务）、tmux 唯一现场、双通道显示、四层单向依赖 | [README](README.md)「架构设计」 · [docs/architecture.md](docs/architecture.md) · issues #5–#7 |
| AI 价值呈现 | Agent 指挥 Agent（平台智能体自主调度 Claude Code）；真实会话逐条实录（10 条工具调用记录） | [docs/evidence/voice-session-transcript.md](docs/evidence/voice-session-transcript.md) · issue #8 |
| AI 工程效率 | 2 天 20 提交从空仓到 3 个 release；M0–M5 首日 6.5 小时完成 | [README](README.md)「工程效率与质量」 · issue #9 · [Actions](https://github.com/huaiyuWangh/Hackathon2026/actions) |

各维度证据与行级指引的完整对照表：[docs/rubric-mapping.md](docs/rubric-mapping.md)。

## 30 秒验证（所有声明可复现）

本仓库的关键声明都能用命令验证，欢迎实际执行：

```sh
# 无音频依赖子集（7 个包，纯 Go）——任何有 Go 1.21 的机器可直接跑
go test ./internal/mcp/ ./internal/ccrunner/ ./internal/config/ \
         ./internal/tools/ ./internal/webui/ ./internal/viewer/ ./internal/brand/

# 全量（9 个包 + race + 覆盖率；internal/linx 为 cgo，需要 portaudio/opus）
# macOS: brew install portaudio opus opusfile
make test          # 应全绿
make cover         # 总覆盖率应为 ~66.7%
make lint          # golangci-lint 应 0 issues

# CI 同款验证在每次 push 时自动执行（.github/workflows/ci.yml），状态见 README 徽章
```

## 仓库结构

```
cmd/bridge/          入口：serve 装配四通道 + --local 调试 CLI
internal/linx/       灵矽设备协议客户端 + 音频(PortAudio/Opus) + MCP Proxy 备用通道
internal/mcp/        MCP 协议分发(JSON-RPC) + HTTP/SSE 传输
internal/tools/      业务工具 dev.claude.send / status / interrupt
internal/ccrunner/   CC 运行时：tmux 驱动 + 任务状态机 + 现场订阅（核心）
internal/viewer/     网页查看器：xterm.js 终端直播(局域网 :8788, 只读)
internal/webui/      麦克风控制页(本机 :8787)
internal/brand/      VoiceCopilot 品牌资产
internal/config/     运行配置(环境变量 + .env)
docs/                架构/路演/演示剧本/协议笔记/证据实录
```

依赖方向严格单向：`linx → mcp → tools → ccrunner`。

## 修改代码前须知（如果你是编码 Agent）

- **凭证**：`.env` 不入库（`.env.example` 是模板），真实 `BRIDGE_API_KEY` 只存在于开发者本机；不要把任何密钥写进代码或文档。
- **cgo**：`internal/linx` 依赖 portaudio/opus（pkg-config）；裸 `go build ./...` 在无音频库的机器上会失败，属预期，不是代码问题。
- **测试**：改动后跑 `make test`（CI 同款 `-race`）；涉及状态的测试一律用 `t.TempDir()`，不要往包目录写 `.bridge/`（历史踩坑）。
- **外部依赖**：`ccrunner` 的 Ensure/Send 需要 `tmux` + `claude` CLI 真实存在，相关测试在缺依赖时自动 skip，勿删守卫。
- **lint**：`make lint` 使用 golangci-lint v2.1.6，配置在 `.golangci.yml`；改配置后先 `golangci-lint config verify`（CI 会做 schema 校验）。
