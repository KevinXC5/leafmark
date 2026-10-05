// Package diagram 解析 Mermaid 流程图并自行完成分层布局与绘制。
//
// 解析（parse.go）与布局（engine.go）不依赖界面库，文字宽度由调用方注入的
// 测量函数给出；只有 paint.go 的字体测量和绘制用到 MyGo 的 ui 包。
package diagram
