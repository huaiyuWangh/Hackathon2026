# Hackathon2026 · 灵矽语音驱动 Claude Code

对着任意灵矽语音入口说一句"帮我修一下失败的测试"，Mac 上的 Claude Code 就开始干活，
大屏（AirPlay）实时看它写代码，结果由语音播报回来。

- **Part 1 语音驱动 CC**（核心）：Mac 桥接程序作为灵矽设备接入（端侧 MCP），
  平台智能体通过工具调用驱动 tmux 会话中的交互式 Claude Code。
- **Part 2 投屏**（展示层）：采用**原生 AirPlay 镜像**（方案A，零开发）；
  网页查看器（方案B）保留为迭代方向。

架构、已确认决策与里程碑详见 [docs/architecture.md](docs/architecture.md)。

## 快速开始

```sh
make build
bin/bridge --local tools list
bin/bridge --local tools call dev.claude.send --arg 'prompt=修复登录测试'
make test
```

> M0 骨架阶段：工具已注册，但 ccrunner 为占位实现，
> 调用会提示 `ccrunner: M1 尚未实现 (tmux 驱动)`，属预期行为。

M1 起为真实链路：`tools call dev.claude.send` 会在 tmux 会话（默认 `ccline`）
里启动交互式 Claude Code 并提交指令，`dev.claude.status` 轮询进度，
完成后摘要里是 CC 的实际回复。想看现场：`tmux attach -t ccline`。

## 项目结构

```
cmd/bridge/       入口：装配 + --local 调试 CLI
internal/linx/    灵矽设备协议客户端（传输层，M3）
internal/mcp/     端侧 MCP 协议分发（JSON-RPC，无业务）
internal/tools/   业务工具 dev.claude.send / status / interrupt
internal/ccrunner/  CC 运行时：tmux 驱动 + 任务状态机（核心，M1）
internal/config/  运行配置（环境变量）
```

依赖方向单向：`linx → mcp → tools → ccrunner`。

## 里程碑

| 阶段 | 内容 | 状态 |
|------|------|------|
| M0 | 骨架 + config + --local CLI | ✅ |
| M1 | ccrunner：tmux 驱动交互式 CC | ✅ |
| M2 | mcp 协议 + tools 本地全链路 | ⬜ |
| M3 | linx 客户端接入灵矽平台（Part 1 闭环） | ⬜ |
| M4 | （迭代）方案B 网页查看器 | ⬜ |
| M5 | （可选）Mac 麦克风音频 / AirPlay 演示环境 | ⬜ |
