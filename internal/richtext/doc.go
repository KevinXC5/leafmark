// Package richtext 把 Markdown 解析成可编辑的块文档。
//
// 坐标一律是 Text() 的 rune 半开区间，块与块之间恰好一个结构换行。
// 未编辑过的块在 Markdown() 里逐字节写回；编辑只重写变脏的块。
package richtext

import "errors"

// Mark 是可叠加的行内样式。
type Mark uint8

const (
	MarkBold Mark = 1 << iota
	MarkItalic
	MarkStrike
	MarkCode
	MarkHighlight
	MarkSub
	MarkSup
	MarkMath // 行内公式，Text 是两个美元符号之间的原文
)

// Kind 是块类型。
type Kind int

const (
	Paragraph Kind = iota
	Heading
	Quote
	List
	Task
	Code
	Horizontal
	TableBlock
	Image
	Raw
	Callout
)

// Align 是表格列对齐。
type Align int

const (
	AlignNone Align = iota
	AlignLeft
	AlignCenter
	AlignRight
)

// Selection 是纯文本上的半开选区。Start 与 End 相等时是光标。
type Selection struct {
	Start int
	End   int
}

// Link 是行内链接。
type Link struct {
	URL   string
	Title string
}

// Run 是一段样式一致的可见文本。Text 按 rune 计偏移。
type Run struct {
	Text  string
	Marks Mark
	Link  *Link
}

// Cell 是表格单元格。
type Cell struct {
	Runs []Run
}

// TableData 是表格。Header 为 true 时第 0 行是表头。
type TableData struct {
	Header bool
	Aligns []Align
	Rows   [][]Cell
}

// CalloutData 保留提示块头部；Fold 为 ""、"+" 或 "-"，标题不占正文坐标。
type CalloutData struct {
	Type  string
	Title string
	Fold  string
}

// Block 是一个顶层块。各字段只对对应 Kind 有意义。
type Block struct {
	Kind    Kind
	Level   int    // 标题 1–6；列表与引用的嵌套深度从 1 起
	Ordered bool   // 有序列表
	Start   int    // 当前有序列表项的显示编号，0 表示 1
	Checked bool   // 任务是否完成
	Lang    string // 代码块语言
	Runs    []Run  // 段落、标题、引用、列表、任务
	Code    string // 代码块正文，可含换行
	Callout *CalloutData
	Table   *TableData
	Alt     string // 图片替代文本
	URL     string // 图片地址
	Title   string // 图片标题
	Raw     string // 未支持语法的原文
}

// ErrRaw 表示编辑落在不可直接修改的 Raw 占位上。
var ErrRaw = errors.New("richtext: 不支持的内容不能直接编辑")

// objectReplacement 是图片、分隔线、Raw 在纯文本里占的一个 rune。
const objectReplacement = "￼"

// Document 是一份可编辑的 Markdown 文档。
type Document struct {
	prefix string // 文档开头未归属任何块的原始空白
	blocks []editBlock
	text   []rune
	index  []span // 与 text 等长，记录每个 rune 落在哪

	undo  []snapshot
	redo  []snapshot
	after Selection // 最近一次编辑完成后的选区，供重做恢复
	// pending 记录折叠光标上待输入的格式，下一次在该点插入时生效。
	pending    *pendingMark
	clean      bool
	coalesce   coalesceKind
	lastChange int // 上一次改变文档的操作序号，用来合并连续输入
	seq        int
}

type coalesceKind int

const (
	coalesceNone coalesceKind = iota
	coalesceInsert
	coalesceDelete
)

// pendingMark 是折叠选区 ToggleMark 记下的待输入样式。
type pendingMark struct {
	at    int
	marks Mark
	set   Mark // 哪些位被显式设定过
	link  *Link
	hasL  bool
}

type span struct {
	block int
	cell  int // 表格单元格在线性化顺序中的下标；-1 表示不是单元格
	run   int
	off   int // 该 rune 在 run.Text 中的下标；占位符为 0
	gap   bool
}

// editBlock 在公开 Block 之外记下源码区间和是否已编辑。
type editBlock struct {
	Block
	source string // 未编辑时的原始片段，含自身尾部换行策略由 gap 负责
	dirty  bool
	gap    string // 与下一块之间的原始空白；最后一块为空
	indent string // 列表项行首的原始缩进，回写嵌套项时沿用
}
