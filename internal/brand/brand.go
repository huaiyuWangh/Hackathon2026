// Package brand 提供 VoiceCopilot 的品牌资产（当前：方形图标 SVG）。
// 注意: embed 无法引用仓库根之外的路径，故 icon.svg 在此存一份，
// 源头版本维护在 docs/brand/logo.svg（两者应保持一致）。
package brand

import (
	_ "embed"
	"net/http"
)

// Name 产品名。
const Name = "VoiceCopilot"

//go:embed icon.svg
var iconSVG []byte

// ServeIcon 把图标挂为 GET（Content-Type 按 SVG）。
func ServeIcon(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml")
	_, _ = w.Write(iconSVG)
}
