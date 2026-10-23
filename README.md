# Hackathon2026 · 灵矽语音驱动 Claude Code

对着任意灵矽语音入口说一句"帮我修一下失败的测试"，Mac 上的 Claude Code 就开始干活，
大屏（AirPlay）实时看它写代码，结果由语音播报回来。

- **Part 1 语音驱动 CC**（核心）：Mac 桥接程序作为灵矽设备接入（端侧 MCP），
  平台智能体通过工具调用驱动 tmux 会话中的交互式 Claude Code。
- **Part 2 投屏**（展示层）：采用**原生 AirPlay 镜像**（方案A，零开发）；
  网页查看器（方案B）保留为迭代方向。

架构、已确认决策与里程碑详见 [docs/architecture.md](docs/architecture.md)。

## 接入真实平台（M3）

```sh
cp .env.example .env   # 填入 BRIDGE_API_KEY（灵矽控制台）
make build
bin/bridge serve       # 上线为灵矓设备, 断线自动重连; BRIDGE_DEBUG=1 看原始消息
```

设备上线后平台会自动发现 `dev.claude.*` 工具；语音闭环还需在灵矽控制台
建智能体并绑定本设备（详见 [docs/linx-protocol-notes.md](docs/linx-protocol-notes.md)）。

## 快速开始

```sh
make build
bin/bridge --local tools list
bin/bridge --local tools call dev.claude.send --arg 'prompt=修复登录测试'
make test
```

平台消息格式模拟（M2）：stdin 逐行喂灵矓外层消息，走完整协议链路——

```sh
echo '{"type":"mcp","payload":{"jsonrpc":"2.0","id":1,"method":"tools/list"}}' | bin/bridge --local sim
echo '{"type":"mcp","payload":{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"dev.claude.send","arguments":{"prompt":"只回复: ok"}}}}' | bin/bridge --local sim
echo '{"type":"mcp","payload":{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"dev.claude.status"}}}' | bin/bridge --local sim
```

> M1 起 `tools call` 为真实链路（tmux 里跑交互式 Claude Code），想看现场：
> `tmux attach -t ccline`（退出按 `Ctrl+b` 再按 `d`）。

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
| M2 | mcp 协议 + tools 本地全链路（--local sim 平台模拟） | ✅ |
| M3 | linx 客户端接入灵矽平台 | 🔶 设备侧✅（真实连接+工具发现），语音闭环待控制台配置 |
| M3 | linx 客户端接入灵矽平台（Part 1 闭环） | ⬜ |
| M4 | （迭代）方案B 网页查看器 | ⬜ |
| M5 | （可选）Mac 麦克风音频 / AirPlay 演示环境 | ⬜ |
