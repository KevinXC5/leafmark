// 步骤描述由 scripts/verify-full.mjs 写成 JSON，交给 internal/desktop/verification_suite.go 逐步执行。
// 步骤函数在 WKWebView 里运行：只取函数源码，不能引用本文件作用域里的变量，可用的名称见 page-helpers.js。

/** 页面步骤：抛出异常即失败，返回值记入结果。extra 可含 shot（截图名）、focus（先激活窗口）、sleep（执行后等待的毫秒数）。 */
export const page = (name, run, extra = {}) => ({ name, run, ...extra });
/** 后端切换工作区，等同于原生“选择文件夹”对话框返回该路径。 */
export const selectFolder = (name, path) => ({ name, selectFolder: path });
/** 在本机执行命令，用于核对磁盘内容；非零退出即失败。 */
export const shell = (name, command) => ({ name, shell: command });
/** 原生点击：locate 在页面里返回目标元素，向其中心投递鼠标按下与抬起。gap 为两者间隔的毫秒数，0 等同触控板“轻触点击”。 */
export const tap = (name, locate, gap = 0) => ({ name, tap: locate, gap, focus: true });
/** 原生悬停：把指针移到 locate 返回的元素中心，之后的页面步骤可以断言 :hover 样式。 */
export const hover = (name, locate) => ({ name, tap: locate, gap: -1, focus: true });
