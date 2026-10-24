# VoiceCopilot 品牌资产

命名定稿：**VoiceCopilot**（2026-10-24）。

## 设计概念

一只**戴耳机的猫**：

- **耳机** = Copilot（副驾驶 / 语音助手的耳朵和嘴）；
- **嘴是一段语音波形** = 它正在听你说、也在把执行结果念给你听（产品的两条语音流）；
- 配色沿用产品终端界面（viewer / 控制页）的 Tokyo Night 色系，logo 与产品界面同源。

## 文件与用途

| 文件 | 用途 |
|------|------|
| `logo.svg` / `png/logo-1024.png` | 方形图标：favicon、GitHub Social Preview、PPT 角标、视频片头 |
| `logo-wordmark.svg` / `png/logo-wordmark-2048.png` | 图标 + 字标横版：README 顶部、PPT 标题页 |
| `logo-banner.svg` / `png/logo-banner-2560.png` | 图标 + 字标 + 一句话 pitch：README 首屏横幅 |

## 色板

| 角色 | 色值 |
|------|------|
| 背景深蓝 | `#1f2335 → #16161e`（渐变） |
| 猫脸 / 主文字 | `#c0caf5` |
| 内耳 | `#bb9af7` |
| 耳机 / 蓝端 | `#7aa2f7` |
| 波形 / 青端（渐变终点） | `#7dcfff` |
| 鼻子 | `#f7768e` |
| 胡须 / 副文字 | `#9aa5ce` |

## 接入位置（待办）

- [ ] README 首屏横幅（issue #2 的 Hero 区）
- [ ] GitHub 仓库 Social Preview（Settings → Social preview，上传 `png/logo-1024.png`）
- [ ] 控制页 `:8787` / 查看器 `:8788/viewer`：favicon + 标题改为 VoiceCopilot
- [ ] 演示视频片头 / 答辩 PPT
