# 方案文档：灵矽语音驱动 Claude Code（Mac 桥接程序）

> 项目一句话：对着灵矽语音入口说一句"帮我修一下失败的测试"，Mac 上的 Claude Code
> 开始干活，大屏上实时看它写代码，结果由语音播报回来。

## 1. 背景与平台

- **灵矽（Xrobot，七牛）**：智能体/设备平台。设备经 WebSocket
  （`ws://xrobo-io.qiniuapi.com/v1/ws/`）或 MQTT 接入；平台提供
  ASR → 智能体 → TTS 的语音闭环，另有长期记忆、知识库、声纹等能力。
- **端侧 MCP（hardware-mcp）**：设备本身作为 MCP Server，JSON-RPC 2.0 消息以
  `{"type":"mcp","payload":{...}}` 封装在设备 WebSocket 内传输。设备在 `hello`
  消息中声明 `features.mcp=true` 后，平台即可通过 `initialize` / `tools/list` /
  `tools/call` 发现并调用设备上的工具。
- **AirPlay**：苹果无线投屏协议。本方案中**仅作单向显示通道**（Mac → Apple TV /
  认证电视），不承载任何控制逻辑。

## 2. 总体架构：三条各司其职的通道

```
┌──────────── Mac（开发机，干活的地方）────────────┐
│  交互式 Claude Code 跑在 tmux 会话（唯一"现场"）    │
│  bridge（本项目，Go 单二进制）:                     │
│    linx 客户端 + 端侧 MCP Server + ccrunner        │
└────┬──────────────────────────┬──────────────────┘
     │ WebSocket                 │ AirPlay 镜像（纯显示）
     │ 语音上行 + MCP 下行         ↓
     ↓                     ┌──────────────┐
┌─ 灵矽平台（云端）─┐         │ 大屏 / Apple TV │
│ ASR → 智能体 → TTS │         │ 实时看 Mac 代码  │
│ 智能体绑定设备工具  │         └──────────────┘
└────┬────────────┘
     │ WebSocket
     ↓
  语音入口：小智硬件 / 灵矽小程序 / Mac 麦克风（M5 可选）
```

- **语音通道**（灵矽设备链路）：说"修复登录测试"→ 平台 ASR → 智能体理解意图。
- **控制通道**（端侧 MCP）：智能体 `tools/call` 下发到 Mac → bridge 驱动 CC 执行
  → 结果返回 → 智能体 TTS 播报摘要（"已修复，3 个用例通过"）。
- **显示通道**（AirPlay）：Mac 画面投到大屏，坐着看 agent 干活。

## 3. 功能拆分（已确认）

### Part 1：语音驱动 CC（核心，产品的全部价值）

不依赖任何屏幕即可完整工作：嘴说指令、耳朵听结果。

- MVP = bridge 注册两三个工具 + 灵矽控制台配一个编程助手智能体。
- **异步工具对**：MCP `tools/call` 是同步请求-响应，撑不住分钟级长任务，因此
  `dev.claude.send` 非阻塞、立即返回任务 ID，`dev.claude.status` 供智能体轮询，
  由智能体决定何时向用户播报；`dev.claude.interrupt` 兜底打断。
- **播报摘要化**：CC 原始输出（diff、日志）没法念，智能体提示词要求把执行结果
  总结成两三句话——细节留给大屏看（嘴说概要，眼看细节）。
- **安全确认**：语音通道等价于可执行任意 shell 的通道，智能体提示词要求危险操作
  （rm、git push、部署等）先语音确认再执行；必要时先 interrupt。

### Part 2：CC 任务执行投屏（展示层，纯加分项）

**决策：采用方案 A——原生 AirPlay 镜像**（Mac 屏幕 → Apple TV / 认证电视）。
零开发，Mac 原生支持。

**保留方案 B（网页查看器）作为迭代方向**：`internal/viewer` + `go:embed` +
xterm.js，从 `ccrunner.Output()` 订阅 `capture-pane` 增量，任何带浏览器的设备
扫码即看。触发条件：演示场地没有 AirPlay 接收端 / 需要手机观看 / 需要文字
清晰度。方案 B 落地时不影响 Part 1 任何代码。

## 4. 耦合点决策：tmux 会话是唯一"现场"（已确认）

CC 以**交互式**跑在固定 tmux 会话（默认 `ccline`）里：

- 语音链路（Part 1）：`tmux send-keys` 写入指令；
- 方案 A（当前）：直接镜像该终端所在的 Mac 屏幕；
- 方案 B（迭代）：`capture-pane` 增量 → channel → WebSocket。

两个 Part 只共享这块"玻璃"，实现上互不牵连。CC 不用 headless
（`claude -p`）方式跑，保证屏幕上始终有东西可看。

## 5. 代码结构（Go，单二进制）

```
cmd/bridge/          入口：装配 + --local 调试 CLI
internal/
  linx/              灵矽设备协议客户端（传输层，无业务）M3 实现
  mcp/               端侧 MCP 协议（JSON-RPC 分发，无业务）
  tools/             业务工具（dev.claude.*，依赖 ccrunner）
  ccrunner/          CC 运行时：tmux 驱动 + 任务状态机 + 输出流 ★核心
  config/            运行配置（环境变量）
  viewer/            （迭代）方案 B 网页查看器
```

依赖方向严格单向：`linx → mcp → tools → ccrunner`；`viewer → ccrunner`。
`mcp` 包只认 `ToolProvider` 接口、不关心传输来源，将来如需把工具挂到灵矽
**云端 MCP**（公网 SSE 端点，任何设备的小智/小程序都能调），只需加一个
SSE 传输层，业务零改动。

**本地调试设计**：`bridge --local tools list|call ...` 绕过灵矽平台直接调用
工具，Part 1 的全部业务逻辑不依赖平台即可自测，联调风险只剩协议层一段。

## 6. 决策记录

| 日期 | 决策 | 状态 |
|------|------|------|
| 2026-10-23 | 功能拆分为 Part 1（语音驱动 CC）+ Part 2（投屏） | 已确认 |
| 2026-10-23 | Part 2 采用方案 A：原生 AirPlay 镜像；方案 B（网页查看器）保留为迭代方向 | 已确认 |
| 2026-10-23 | 耦合点：tmux 会话是唯一现场，CC 以交互式运行其中 | 已确认 |
| 2026-10-23 | 实现语言 Go，单二进制 bridge | 已确认 |
| 2026-10-23 | Part 1 采用异步工具对（send/status/interrupt） | 设计定稿 |
| 2026-10-23 | 工具协议层传输无关（ToolProvider 接口），预留云端 MCP 出口 | 设计定稿 |

## 7. 里程碑

| 阶段 | 内容 | 验收方式 | 状态 |
|------|------|----------|------|
| M0 | 骨架 + config + --local CLI | `bridge --local tools list` 列出工具 | ✅ |
| M1 | ccrunner：tmux 驱动交互式 CC、状态机、输出流 | `--local tools call dev.claude.send` 真跑（fake 程序 + 真实 CC 双冒烟通过） | ✅ |
| M2 | mcp 协议 + tools 全链路（本地） | `--local` 全链路 + 单测 | ⬜ |
| M3 | linx 客户端接入灵矽平台，控制台配智能体 | 小智/小程序语音驱动 CC | ⬜ |
| M4 | （迭代）方案 B 网页查看器 internal/viewer | 手机浏览器实时看输出 | ⬜ |
| M5 | （可选）Mac 麦克风音频入口 / AirPlay 演示环境 | 现场演示 | ⬜ |

## 8. M1 实现要点（2026-10-23）

- **TUI 就绪检测**：只等 node 进程稳定不够——欢迎屏渲染期间 send-keys 的字符
  会被启动过程吞掉（冒烟实测踩坑）。Ensure 在进程稳定后继续轮询画面出现输入框
  标记 `❯` 才放行发送。
- **完成检测启发式**：CC 工作中页脚显示 "Esc to interrupt"（busy 标记）；
  标记消失 + 画面连续 ~2s 无变化 → 任务 done；见过 busy 后最短等待 1.5s，
  从未见过 busy 最短 3s（防"刚下发就误判"）。所有常量集中在
  `ccrunner/tmux.go` 顶部，随 CC 版本演进调参。
- **跨进程状态机**：任务与采样基线持久化在工作目录 `.bridge/state.json`
  （已 gitignore）。`--local` 下 send/status 是不同进程，status 每次调用先
  同步采样一拍再返回；采样间隔 ≥2s 且画面未变时直接视为稳定。
- **播报摘要**：`summarize` 从画面末尾取非 UI 装饰行（过滤边框/块元素 logo/
  欢迎屏/页脚/空输入框），只留对话内容（如 `❯ 指令 / ⏺ 回复`）。
- **输出流**：monitor 每 500ms capture-pane（ANSI），前缀差分出增量推送
  `Output()` channel，无消费者时丢弃——为 M4 方案B 查看器预留。
- **验证记录**：fake 回显程序（send-keys/跨进程状态机）+ 真实 Claude Code
  v2.1.88（下发→回复→done 摘要含 `⏺ ok`/`⏺ good`）双冒烟通过。

## 9. 风险与对策

| 风险 | 对策 |
|------|------|
| CC 任务耗时分钟级，MCP 同步调用超时 | 异步工具对：send 立即返回，status 轮询 |
| 语音播报冗长/不可念 | 智能体提示词强制摘要化；细节上大屏看 |
| 语音通道可执行任意命令 | interrupt 工具 + 提示词要求危险操作先确认 |
| 现场 AirPlay 连不上（WiFi 隔离等） | Part 1 语音闭环照演；降级用笔记本浏览器（方案 B） |
| 灵矽协议联调问题 | --local 模式把业务逻辑全部前置自测，联调只剩传输层 |
