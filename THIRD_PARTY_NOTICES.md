# 第三方声明

Leafmark 以 MIT 许可证分发，见仓库根目录的 [LICENSE](LICENSE)。安装包同时包含下列第三方软件和字体。使用、修改或再分发时，须保留各自的版权声明和许可证。

本文件只列出直接依赖和会进入安装包的主要组件，不构成完整的传递依赖清单。各组件的完整许可证文本以其上游仓库为准。

## 桌面框架与运行时

| 组件 | 许可证 | 来源 |
| --- | --- | --- |
| MyGo 0.2.8 | MIT | https://github.com/egoist/mygo |
| purego | Apache-2.0 | https://github.com/ebitengine/purego |
| golang.org/x/image | BSD-3-Clause | https://cs.opensource.google/go/x/image |
| go-text/typesetting | Unlicense OR BSD-3-Clause | https://github.com/go-text/typesetting |

go-text/typesetting 随 MyGo 的文字排版进入桌面应用，版权归 The go-text authors（2021）。MyGo 的命令行工具只在构建时使用。Go 工具链本身按 BSD 风格许可证分发，不随 Leafmark 安装包提供。

## 文档模型

| 组件 | 许可证 | 来源 |
| --- | --- | --- |
| goldmark 1.8.6 | MIT | https://github.com/yuin/goldmark |

goldmark 版权归 Yusuke Inuzuka（2019）。桌面应用用它识别 Markdown 结构，并在导出 HTML 时渲染保留为原文的块；未编辑的原文由 Leafmark 自己写回。

## 字体与图标

下列字体嵌入桌面应用，许可证均为 SIL Open Font License 1.1：

- Inter：Copyright 2016 The Inter Project Authors
- Newsreader：Copyright 2020 The Newsreader Project Authors
- Geist Mono：Copyright 2024 Vercel, Inc.
- STIX Two Math：Copyright 2001–2021 The STIX Fonts Project Authors，用于公式排版

Inter、Newsreader 和 Geist Mono 的完整许可证保存在 `internal/desktop/fonts/`，STIX Two Math 的许可证保存在 `internal/mathlayout/fonts/`。再分发桌面应用时须保留上述版权与许可证，字体不得单独改名后当作原字体销售。中文及未覆盖字形回退到操作系统字体。官网使用 Inter、Newsreader 和 Geist Mono 的 WOFF2 文件，同样遵循 SIL Open Font License 1.1。

Lucide 图标用于桌面界面，遵循 ISC 许可证，来源为 https://github.com/lucide-icons/lucide。
