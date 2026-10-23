# Hackathon2026 · 灵矽语音驱动 Claude Code

对着任意灵矽语音入口说一句"帮我修一下失败的测试"，Mac 上的 Claude Code 就开始干活，
大屏（AirPlay）实时看它写代码，结果由语音播报回来。

- **Part 1 语音驱动 CC**（核心）：Mac 桥接程序作为灵矽设备接入（端侧 MCP），
  平台智能体通过工具调用驱动 tmux 会话中的交互式 Claude Code。
- **Part 2 投屏**（展示层）：采用**原生 AirPlay 镜像**（方案A，零开发）；
  网页查看器（方案B）保留为迭代方向。

架构、已确认决策与里程碑详见 [docs/architecture.md](docs/architecture.md)。

## 接入真实平台（M3+M5 语音闭环）

```sh
cp .env.example .env   # 填入 BRIDGE_API_KEY（灵矽控制台）；BRIDGE_AUDIO=1 开语音
brew install portaudio opus opusfile   # 音频依赖（一次性）
make build

bin/bridge --local mic-test   # 首次运行触发麦克风授权弹窗, 确认"正常收音"
bin/bridge serve              # 在你自己的终端里跑（麦克风权限归属终端 App）
```

启动后：

- **控制页** http://127.0.0.1:8787/ —— 大按钮开关麦克风（默认关闭），点亮后对着 Mac 说话
- 语音链路：Mac 麦克风 → 灵矽 ASR/智能体（realtime 模式，服务端 VAD）→
  端侧工具 `dev.claude.*` → tmux 里的 Claude Code 干活 → 结果 TTS 从 Mac 播报
- 看现场：`tmux attach -t ccline`（退出 `Ctrl+b` 再按 `d`）

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
cmd/bridge/       入口：装配 + --local 调试 CLI（tools/sim/mic-test）
internal/linx/    灵矽设备协议客户端 + 音频(PortAudio/Opus) + MCP Proxy 备用通道
internal/mcp/     MCP 协议分发（JSON-RPC）+ HTTP/SSE 传输
internal/tools/   业务工具 dev.claude.send / status / interrupt
internal/ccrunner/  CC 运行时：tmux 驱动 + 任务状态机（核心）
internal/webui/   麦克风控制页（go:embed 单页）
internal/config/  运行配置（环境变量 + .env）
```

依赖方向单向：`linx → mcp → tools → ccrunner`。

## 里程碑

| 阶段 | 内容 | 状态 |
|------|------|------|
| M0 | 骨架 + config + --local CLI | ✅ |
| M1 | ccrunner：tmux 驱动交互式 CC | ✅ |
| M2 | mcp 协议 + tools 本地全链路（--local sim 平台模拟） | ✅ |
| M3 | linx 客户端接入灵矓平台 + 智能体联调 | ✅ |
| M4 | （迭代）方案B 网页查看器 | ⬜ |
| M5 | Mac 音频入口（realtime）+ 控制页 | 🔶 待端到端验证 |
