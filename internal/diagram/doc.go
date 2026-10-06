// Package diagram 解析 Mermaid 图表并自行完成布局与绘制。
//
// 支持流程图、时序图、甘特图、饼图、类图、状态图与 ER 图的常用语法子集，
// 其他图种由 Parse 返回错误。
//
// 解析（parse.go、kinds*.go）与布局（engine.go、special_layout.go）不依赖界面库，
// 文字宽度由调用方注入的测量函数给出。布局结果经 draw.go 变成一组与设备无关的
// 绘制指令，paint.go 把它们画到 MyGo 的 Painter 上，svg.go 把它们写成 SVG，
// 两边的坐标与配色因此保持一致。
package diagram
