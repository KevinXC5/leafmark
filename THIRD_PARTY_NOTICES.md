# 第三方声明

Leafmark 以 MIT 许可证分发，见仓库根目录的 [LICENSE](LICENSE)。安装包同时包含下列第三方软件和字体。使用、修改或再分发时，须保留各自的版权声明和许可证；字体还须遵守 SIL Open Font License 1.1，不得单独把字体文件改名后当作原字体销售。

本文件只列出直接依赖和会进入安装包的主要组件，不构成完整的传递依赖清单。各组件的完整许可证文本以其上游仓库为准。

## 桌面框架与运行时

| 组件 | 许可证 | 来源 |
| --- | --- | --- |
| MyGo | MIT | https://github.com/egoist/mygo |
| mygo-runtime | MIT | https://www.npmjs.com/package/mygo-runtime |
| mygo-cli | MIT | https://www.npmjs.com/package/mygo-cli |
| purego | Apache-2.0 | https://github.com/ebitengine/purego |
| golang.org/x/image | BSD-3-Clause | https://cs.opensource.google/go/x/image |

Go 工具链本身按 BSD 风格许可证分发，不随 Leafmark 安装包提供。

## 编辑、渲染与界面

| 组件 | 许可证 |
| --- | --- |
| CodeMirror 6（`@codemirror/*`、`@lezer/markdown`） | MIT |
| markdown-it 及其插件 | MIT |
| DOMPurify | MPL-2.0 OR Apache-2.0 |
| highlight.js | BSD-3-Clause |
| KaTeX | MIT |
| Mermaid | MIT |
| elkjs（Mermaid 的布局依赖） | EPL-2.0 |
| Lucide | ISC |

## 字体

Inter、Newsreader 和 Geist Mono 通过 `@fontsource` 引入，许可证均为 SIL Open Font License 1.1。

- Inter：Copyright 2016 The Inter Project Authors
- Newsreader：Copyright 2020 The Newsreader Project Authors
- Geist Mono：Copyright 2024 Vercel, Inc.

## 仅用于开发

TypeScript 为 Apache-2.0，Vite、Bun 测试相关依赖和 jsdom 为 MIT。这些工具不随安装包分发给最终用户。
