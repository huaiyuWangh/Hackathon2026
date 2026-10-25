# 评审维度 → 证据对照表

> 本表把每个评审维度的证据精确到文件与章节，替评审完成检索工作。
> 所有声明均可用命令复现（见 [AGENTS.md](../AGENTS.md)「30 秒验证」）。

## 总览

| 维度 | 权重 | 最强证据（一句话） | 入口 |
|---|---|---|---|
| 商业价值 & 产品设计 | 50% | 无键盘编程刚需场景（通勤修 CI / 无障碍），有完整商业画布与 M6–M10 Roadmap | [README](../README.md) · [pitch.md](pitch.md) |
| 架构设计 | 20% | 端侧 MCP（设备即工具服务）+ tmux 唯一现场 + 四层单向依赖 + 双端口安全隔离 | [architecture.md](architecture.md) |
| AI 价值呈现 | 15% | Agent 指挥 Agent：平台智能体自主决策调用端侧工具驱动 Claude Code，有平台原始会话实录 | [voice-session-transcript.md](evidence/voice-session-transcript.md) |
| AI 工程效率 | 15% | 空仓到 3 个 release 共 2 天 20 提交，M0–M5 首日 6.5 小时；覆盖率 66.7%、CI/race/lint 全绿 | [issue #9](https://github.com/huaiyuWangh/Hackathon2026/issues/9) · [Actions](https://github.com/huaiyuWangh/Hackathon2026/actions/workflows/ci.yml) |

## 商业价值 & 产品设计（50%）

- **痛点与场景**：[README](../README.md)「为什么需要它」「一个场景」（地铁修 CI 场景）、「三大能力」
- **产品定位与命名**：[issue #1](https://github.com/huaiyuWangh/Hackathon2026/issues/1)（VoiceCopilot 定稿过程与理由）
- **商业画布**：[pitch.md](pitch.md)——用户价值、竞品差异（语音入口天然免键盘）、商业模式（技能平台演进）、Roadmap M6–M10
- **品牌**：[docs/brand/](brand/)（logo 三件套与色板），产品页/查看器/控制页全链路接入
- **产品化文档**：[demo-script.md](demo-script.md)（3 分钟演示剧本，含现场故障预案）

## 架构设计（20%）

- **总体架构**：[architecture.md](architecture.md)（决策记录式，含备选方案对比）
- **两张图**：README「一图看懂」——三通道架构图（语音/控制/显示）+ 双 Agent 时序图
- **关键决策**：
  - 端侧 MCP 而非云端 MCP（设备即工具服务，无需公网；详见 issue #5 与 [linx-protocol-notes.md](linx-protocol-notes.md)）
  - tmux 会话是唯一"现场"（语音写入 / AirPlay 镜像 / viewer 订阅同源）
  - 异步工具对 send/status/interrupt 撑住分钟级长任务（issue #7）
  - 双端口隔离：控制面 127.0.0.1:8787 不出本机，只读直播 0.0.0.0:8788（issue #6）
- **代码佐证**：依赖单向 `linx → mcp → tools → ccrunner`（9 包，`go list` 可验证）；`internal/mcp` 只依赖 `ToolProvider` 接口，协议与业务解耦

## AI 价值呈现（15%）

- **核心叙事：Agent 指挥 Agent**——灵矽平台智能体（调度）↔ Claude Code（执行），人只负责说和听；README 双 Agent 时序图
- **真实会话实录**（最强证据）：[voice-session-transcript.md](evidence/voice-session-transcript.md)
  - 平台 chat-history API 原始记录，2026-10-23，31 条消息：5 轮用户语音、10 条工具调用记录
  - 证明：智能体**自主决定**调 `dev.claude_send`；异步轮询 `dev.claude_status`；**多轮上下文**（第三问直接复用前次分析、未再调工具）；结果拆句 TTS 播报
- **AI 元素盘点与话术**：[issue #8](https://github.com/huaiyuWangh/Hackathon2026/issues/8)
- **演示视频**（人类评审）：README 顶部两支——语音端到端 77s、手机查看器

## AI 工程效率（15%）

- **速度证据**：git 提交时间线（README「工程效率与质量」有逐条链接）——
  2026-10-23 14:11 空仓初始化 → 20:28 M0–M5 全部完成且**端到端验证通过**（净 6.5 小时）；
  次日完成 M4 查看器、品牌、产品化、工程化补强，共 2 天 20 个提交、3 个 release
- **质量证据**：
  - 覆盖率 34.7% → **66.7%**（codecov 徽章实时可查）；9 个包 68 个测试函数，`go test -race` 全绿
  - CI 双 job（test+lint）全绿：[Actions](https://github.com/huaiyuWangh/Hackathon2026/actions/workflows/ci.yml)；golangci-lint 0 issues
  - 3 个 release（v0.1.0 闭环 / v0.2.0 查看器 / v0.3.0 产品定稿+工程化），均附可运行二进制
- **人机分工记录**：[issue #9](https://github.com/huaiyuWangh/Hackathon2026/issues/9)（AI 证据包）
