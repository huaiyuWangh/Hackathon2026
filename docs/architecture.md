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
| M2 | mcp 协议 + tools 全链路（本地） | `--local sim` 用灵矓外层消息格式驱动真实 CC 全链路通过 + 单测 | ✅ |
| M3 | linx 客户端接入灵矓平台，控制台配智能体 | 设备侧✅：连接/握手/initialize+tools/list 通过；智能体侧联调完成（见 §10.1） | ✅ |
| M4 | （迭代）方案 B 网页查看器 internal/viewer | 手机浏览器实时看输出 | ⬜ |
| M5 | Mac 音频入口（realtime 模式）+ 控制页 | 对着 Mac 说话驱动 CC（待用户麦克风授权后端到端验证） | 🔶 |

## 8. M1 实现要点（2026-10-23）

- **TUI 就绪检测**：只等 node 进程稳定或画面出现输入框 `❯` 都不够——欢迎屏的
  输入框先于 Ink 输入处理就绪，提前 send-keys 会被吞掉（M1/M2 冒烟两次踩坑）。
  最终方案是**按键往返探测**：新建会话时键入一个字符，等它真实回显在输入框里
  再退格删除，之后才放行正式发送；已存在会话走 fast-path 不打扰用户输入。
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

## 9. M2 实现要点（2026-10-23）

- **linx.Router**：外层消息（`{"type":"mcp","payload":{...}}`）→ JSON-RPC →
  mcp.Server 的纯编解码分发，可脱离 WebSocket 单测/模拟；通知（无 id）不回包，
  坏 payload 回 -32700，非 mcp 类型留给 M3 音频/状态处理。
- **`bridge --local sim`**：平台下行模拟器。stdin 逐行读外层消息，走完整协议
  链路（Envelope→JSON-RPC→mcp→tools→ccrunner→真实 CC）后把响应外层消息写回
  stdout。与 M3 的差别仅在传输层：把 stdin/stdout 换成设备 WebSocket。
- **协议细节对齐灵矓文档**：tools/list 增加推荐格式的 `type` 字段（0=tool 函数）；
  未知工具错误映射 JSON-RPC -32601（文档示例错误码）；inputSchema 无必填参数时
  省略 `required`（不输出 null）。
- **serve 装配预就绪**：`bridge serve` 已完成 runner→registry→linx.New 装配，
  M3 只需实现 Client.Run 的 WebSocket 循环。
- **验证记录**：sim 模式下 initialize / tools/list（含 type 字段）/ 未知工具
  -32601 / send→真实 CC→done（摘要 `⏺ m2链路通`）全部通过。

## 10. M3 实现要点（2026-10-23）

- **连接与鉴权**：`wss://xrobo-io.qiniuapi.com/v1/ws/`，握手带 4 个 header：
  `Authorization: Bearer`（.env 的 BRIDGE_API_KEY）、`Protocol-Version: 1`、
  `Device-Id`（自动探测 en0 MAC，可用 BRIDGE_DEVICE_ID 覆盖）、`Client-Id`
  （自动生成并持久化在 `.bridge/client-id`，重连保持不变）。
- **hello 握手**：`{version:1, features:{mcp:true}, transport:"websocket",
  audio_params:{}}`，等服务端 hello（10s 超时），session_id 记日志。
- **保活与重连**：无应用层心跳，30s WebSocket ping + pong 续期读超时（90s）；
  断线指数退避重连（1s→30s 封顶）。会话恢复靠异步工具对：智能体重连后调
  dev.claude.status 拿回现场。
- **.env 装载**：config 启动时读工作目录 `.env`（已 gitignore），已设置的
  环境变量优先；`BRIDGE_DEBUG=1` 打印原始收发消息。
- **真实平台验证记录（2026-10-23）**：连接成功 → 握手 session 下发 →
  **平台主动** `initialize`（clientInfo=XiaozhiClient, protocolVersion=2024-11-05）
  → `tools/list` → 我方应答三个 dev.claude.* 工具，25s 稳定无断连。
  结论：端侧工具**无需控制台预先配置**即被平台发现；
  Bearer 凭证（lx- key）有效；自动探测 MAC 被接受。
- **待控制台完成**（语音闭环最后一步，见 linx-protocol-notes.md 待确认清单）：
  建智能体并绑定本设备（MAC f0:2f:4b:02:d7:5a）、勾选 MCP 服务、
  验证多轮工具调用（send → status 轮询 → 播报）。

## 10.1 M3 智能体侧联调记录（2026-10-23 晚）

用户在控制台建好智能体（Hackathon2026，绑定本机）后逐轮排障，关键结论：

- **会话锚定规则（核心发现）**：只有**锚定到设备的会话**才会挂载该设备的端侧
  工具。控制台"网页体验"/智能体直聊的会话 macAddress=智能体ID，调用工具报
  `Tool dev.claude.send not found in plugin registry`（设备工具不在其插件注册表）。
  灵矽没有独立 App，**语音入口就是设备本身**——因此 Mac 必须自己当语音设备
  （见 §10.2），这也是放弃云端 SSE 方向、回归端侧 MCP 的原因。
- **设备工具描述（控制台层）**：设备上报的工具描述只用于协议层发现；智能体
  上下文注入的前置提示词要在「设备工具描述 API」（POST /v1/device/tools，
  pre_prompt_texts 按语言）注册。已注册三个工具的 zh 前置提示词。
- **智能体系统提示词**：默认是陪伴型模板（"台湾女孩"人设），永不调工具——
  已通过 PUT /xiaozhi/agent/{id} 替换为编程助手人设（工作流程/摘要播报/危险
  操作确认，见 §3），替换后智能体立即尝试 dev.claude.send（聊天记录实证）。
- **意图模型**：Intent_function_call 已启用（智能体详情确认）。
- **已否决的弯路**：MCP 市场注册私有 SSE 服务（POST /v1/mcp-store/servers）
  在 config 校验阶段反复 "invalid mcp config"（SSE 端点可达、工具可拉取仍被拒），
  且与端侧方向不符，已放弃；cloudflared 隧道停用。SSE 传输层代码保留
  （internal/mcp/sse.go，本地验证可用）；MCP Proxy 探明为"平台侧作 MCP 客户端
  的 WS 代理"（连接即发 initialize），ProxyClient 保留为备用通道。

## 10.2 M5 端侧语音（Mac 即语音设备，提前实施）

- **音频链路**：PortAudio（麦克风/扬声器，cgo）+ libopus；上行 16kHz 单声道
  60ms Opus 帧，下行 TTS Opus 解码播放，`tts stop is_aborted=true` 清播放缓冲。
  realtime 模式（`listen start`）持续上行，**VAD 与人声打断都在服务端**，设备端
  无需唤醒词/本地 VAD。
- **audio_params 坑**：hello 的 audio_params.format 必须是**字符串** "opus"
  （小智协议惯例）；传数组服务端直接不回 hello（握手超时）。服务端 hello 会
  协商回完整参数（含 play_buffer_duration=600）。
- **控制页**（internal/webui，go:embed 单页）：http://127.0.0.1:8787/ 中间大
  按钮开关麦克风；默认静音，点击后上行才生效。API：POST /api/mic、GET /api/status。
- **麦克风权限坑（macOS TCC）**：无权限时系统**静默给全零音频**，无弹窗无报错，
  服务端永远听不到。排查工具：`bridge --local mic-test`（录 5s 打印峰值）。
  正确姿势：在用户自己的终端 App 里跑 serve（首次访问麦克风会触发授权弹窗）。
- 依赖：`brew install portaudio opus opusfile`；Makefile 已内置
  PKG_CONFIG_PATH（MacPorts 的 pkg-config 看不到 /usr/local）。

## 11. 风险与对策

| 风险 | 对策 |
|------|------|
| CC 任务耗时分钟级，MCP 同步调用超时 | 异步工具对：send 立即返回，status 轮询 |
| 语音播报冗长/不可念 | 智能体提示词强制摘要化；细节上大屏看 |
| 语音通道可执行任意命令 | interrupt 工具 + 提示词要求危险操作先确认 |
| 现场 AirPlay 连不上（WiFi 隔离等） | Part 1 语音闭环照演；降级用笔记本浏览器（方案 B） |
| 灵矽协议联调问题 | --local 模式把业务逻辑全部前置自测，联调只剩传输层 |
