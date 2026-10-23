# 灵矽接入协议笔记（linx client 实现依据）

> 来源：灵矽官方文档（2026-10-23 抓取），联调结论随手补在本文件。
> 文档中心：https://linx.qiniu.com/docs
> · WebSocket 协议: /docs/xrobot/platform/websocket
> · 端侧 MCP: /docs/xrobot/mcp/hardware-mcp
> · 智能体配置: /docs/xrobot/guide/agent-config

## 1. 连接与鉴权（已确认：走 HTTP header，不走 URL、不走 hello）

- 地址：`wss://xrobo-io.qiniuapi.com/v1/ws/`（推荐 WSS；`ws://` 为内部网络用。
  **config 默认值需从 ws:// 改为 wss://**）
- 握手 header（4 个）：
  - `Authorization: Bearer <token>`
  - `Protocol-Version: 1`（需与 hello 中 version 一致）
  - `Device-Id: <MAC 地址>`（物理网卡 MAC，平台用于验证设备绑定）
  - `Client-Id: <UUID>`（软件生成，bridge 可启动时生成并持久化）
- 本机（wanghuaiyu 的 Mac）：en0 = `f0:2f:4b:02:d7:5a`
- ⚠️ 待联调确认：Bearer token 用 tk_ ID 还是 lx- Key（或需换取）；
  控制台设备绑定处登记的 MAC 是否须与 Device-Id 一致。

## 2. hello 握手（比 M0 骨架多 version/transport/audio_params）

```json
{
  "type": "hello",
  "version": 1,
  "features": { "mcp": true },
  "transport": "websocket",
  "audio_params": {}
}
```

- 服务端回 `type:"hello"` + `session_id`（可选）+ 协商 audio_params。
- 等待 hello 超时 10s。
- 纯 MCP 设备 `audio_params` 可为空对象（端侧 MCP 文档示例即空）。
- `features.notify: true` 可选，用于接收 `notify: config_updated`。
- `agent_params.custom_replace_prompt` 可替换智能体提示词占位符（暂不用）。
- **DeviceID 不在 hello 里，在 header 里** → protocol.go 的 Hello 结构需补
  version/transport/audio_params、去掉 DeviceID；config.DeviceID 语义改为
  Device-Id header（应为 MAC）。

## 3. 心跳与生命周期

- 官方文档**无应用层心跳**：依赖标准 WebSocket ping/pong + 断线重连。
- 无 goodbye 消息：会话结束 = 直接关闭 WebSocket。
- 异步工具对兜底：重连后智能体再调 dev.claude.status 恢复现场。

## 4. 端侧 MCP 消息封装（与 internal/mcp 现有假设一致 ✅）

外层 `{"type":"mcp","payload":{JSON-RPC 2.0}}`，protocolVersion `2024-11-05`。

平台 → 设备：
- `initialize`（params 带 clientInfo/capabilities；vision 能力可忽略）
- `tools/list`（首次无 params，分页带 cursor）
- `tools/call`（params: name + arguments）

设备 → 平台：
- initialize 响应：result.protocolVersion / capabilities `{"tools":{}}` / serverInfo
- tools/call 响应：`result.content: [{type:"text",text:...}]` + `isError`；
  失败走 JSON-RPC `error`（如 -32601 未知工具）
- 工具命名建议"模块.功能"（dev.claude.send ✅ 符合）
- ⚠️ 文档未写 tools/call 超时时长 → 更支持 send 异步立即返回的设计

## 5. 需忽略的服务端消息（不能崩）

`stt`（ASR 文本）、`llm`（emotion/action）、`tts`（start/stop/sentence_start）、
`notify`。设备端消息 `listen`/`abort` 我们基本不发（无音频）。

## 6. 控制台侧配置项（agent-config 文档 + 2026-10-23 二次确认）

智能体配置页 = 顶部工具栏（右上「保存配置」）+ 中间配置卡片（点开右侧抽屉）。
- 提示词 → 「基础信息」→「角色介绍」（≤6000 字）
- MCP → 「技能与扩展」→「MCP 编辑」：**这里勾的是云端 SSE 市场服务；
  端侧设备工具不需要在此勾选**（设备上线即被 initialize + tools/list 发现）
- ⚠️ 意图识别模式选「无意图识别」会**隐藏 MCP 编辑入口且不调用技能**，
  必须选「统一意图识别」（需模型支持工具调用）
- 空闲超时 → 「对话与视觉」，默认 120s，范围 60~3600s
- 声纹（加分项）→ 「声纹功能」：添加说话人 + 仅回应已添加说话人
- 各配置保存位置不一：主表单右上「保存配置」；MCP 在弹窗内独立「保存配置」；
  保存后需**断开重连**才对新会话生效
- 设备绑定见「快速入门与设备绑定」章节（待查）

## 7. 待确认清单（联调时逐项打勾）

- [x] Bearer token 到底用 tk_ 还是 lx-（或控制台另发token）→ **2026-10-23 实测：.env 中的 lx- key 连接成功**
- [x] 端侧 MCP 工具是否需在控制台「MCP 编辑」里出现/勾选，还是设备绑定后自动可见
      → **实测：设备上线后平台主动 initialize + tools/list，无需预先配置即被发现**
- [x] protocolVersion → **实测：平台 initialize 带 `2024-11-05`，与实现一致**
- [x] Device-Id 用 en0 MAC → **实测：自动探测的 en0 MAC 被接受，握手成功**
- [x] 控制台设备绑定是否要求 MAC 一致 → 2026-10-23 控制台已按 en0 MAC
      (f0:2f:4b:02:d7:5a) 绑定设备，serve 实连握手成功
- [ ] 智能体能否多轮工具调用（send → status 轮询 → 播报）★最大产品风险
- [ ] tools/call 平台侧超时时长
- [ ] 同 Device-Id 重复连接是否互踢（serve 在跑时勿再起第二个实例）

## 8. 2026-10-23 真实平台联调记录（bridge serve, BRIDGE_DEBUG=1）

```
已连接 wss://xrobo-io.qiniuapi.com/v1/ws/ (device f0:2f:4b:02:d7:5a client cda1…)
握手完成 session=_K8AALV1GfJ8XgYA
<< initialize (clientInfo=XiaozhiClient, protocolVersion=2024-11-05, 带 vision 能力)
<< tools/list (id=2, 无 params)
>> initialize 响应 + tools/list 响应（3 个 dev.claude.* 工具）
…25s 稳定无断连
```

平台在设备上线后**立即**主动 initialize + tools/list（无需任何触发）。
vision 能力（url+token）我们按需忽略。
