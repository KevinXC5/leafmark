package diagram

// 图种，对应 Mermaid 源码的首个关键字。Graph.Kind 为空时与 KindFlowchart 等价。
const (
	KindFlowchart = "flowchart"
	KindSequence  = "sequenceDiagram"
	KindGantt     = "gantt"
	KindPie       = "pie"
	KindClass     = "classDiagram"
	KindState     = "stateDiagram"
	KindER        = "erDiagram"
)

// Direction 是流程图的走向。
type Direction uint8

const (
	TopDown Direction = iota
	BottomUp
	LeftRight
	RightLeft
)

// Shape 是节点的外形。
type Shape uint8

const (
	ShapeRect             Shape = iota // A[文字]，未写形状时也是它
	ShapeRound                         // A(文字)
	ShapeStadium                       // A([文字])
	ShapeCircle                        // A((文字))
	ShapeDiamond                       // A{文字}
	ShapeHexagon                       // A{{文字}}
	ShapeCylinder                      // A[(文字)]
	ShapeParallelogram                 // A[/文字/]
	ShapeParallelogramAlt              // A[\文字\]
	ShapeTrapezoid                     // A[/文字\]
	ShapeTrapezoidAlt                  // A[\文字/]
	ShapeFlag                          // A>文字]
	ShapeSubroutine                    // A[[文字]]
	ShapeDoubleCircle                  // A(((文字)))
	ShapeNote                          // 类图与状态图的备注
	ShapeState                         // 状态图的状态
	ShapeStart                         // 状态图的起点 [*]
	ShapeEnd                           // 状态图的终点 [*]
	ShapeBar                           // 状态图的 fork / join
	ShapeClass                         // 类图的类：名称、属性、方法三栏
	ShapeEntity                        // ER 图的实体：名称加属性表
)

// LineStyle 是连线的线型。
type LineStyle uint8

const (
	LineSolid     LineStyle = iota // -->
	LineDotted                     // -.->
	LineThick                      // ==>
	LineInvisible                  // ~~~，只参与布局
)

// Arrow 是连线端点的标记。
type Arrow uint8

const (
	ArrowNone          Arrow = iota
	ArrowNormal              // >
	ArrowCircle              // o
	ArrowCross               // x
	ArrowOpen                // 类图的关联、依赖：两笔开口箭头
	ArrowTriangle            // 类图的继承、实现：空心三角
	ArrowDiamond             // 类图的聚合：空心菱形
	ArrowDiamondFilled       // 类图的组合：实心菱形
	ArrowOne                 // ER 图：有且仅有一个 ||
	ArrowZeroOrOne           // ER 图：零或一个 |o
	ArrowOneOrMany           // ER 图：一个或多个 }|
	ArrowZeroOrMany          // ER 图：零或多个 }o
)

// Graph 是解析后的图表。流程图、类图、状态图与 ER 图用 Nodes、Edges、Subgraphs 描述；
// 时序图的 Nodes 是参与者，消息等内容与甘特图、饼图的数据保存在内部字段里。
type Graph struct {
	// Kind 是图种；空值与 KindFlowchart 等价。
	Kind      string
	Title     string
	Direction Direction
	Nodes     []*Node
	Edges     []*Edge
	Subgraphs []*Subgraph

	classes  map[string]Attributes
	sequence []sequenceEvent
	boxes    []sequenceBox
	tasks    []ganttTask
	slices   []pieSlice
	showData bool
}

// Node 是一个节点。Text 可含换行，未定义文字时等于 ID。
type Node struct {
	ID           string
	Text         string
	Shape        Shape
	Style        Attributes
	Classes      []string
	URL, Tooltip string
	// Members 是类图的成员或 ER 图的属性，每项一行。
	Members []string

	group      int        // 所属子图在 Graph.Subgraphs 中的下标，-1 表示不属于任何子图
	annotation string     // 类图的 <<interface>> 之类
	attrs      []string   // 类图的属性栏
	methods    []string   // 类图的方法栏
	rows       [][]string // ER 图的属性行：类型、名称、键、说明
	actor      bool       // 时序图里用 actor 声明的参与者
}

// Edge 是一条连线。Length 是它至少跨过的层数，--> 为 1，---> 为 2。
type Edge struct {
	From, To   string
	Label      string
	Line       LineStyle
	Head, Tail Arrow
	Length     int
	Style      Attributes
	// Relation 是类图与 ER 图里关系的原始写法，如 <|-- 或 ||--o{。
	Relation string
	// CardinalityFrom 与 CardinalityTo 是类图关系两端的数量标注。
	CardinalityFrom, CardinalityTo string

	from, to       int // 端点在 Graph.Nodes 中的下标
	fromSub, toSub int // 端点写的是子图 ID 时，是该子图的下标加一；否则为 0
}

// Subgraph 是一个子图（状态图里是复合状态，类图里是命名空间）。
// Parent 是外层子图的下标，-1 表示顶层；Nodes 只列出直接属于它的节点。
// HasDirection 为真时子图内部按 Direction 排布。
type Subgraph struct {
	ID           string
	Title        string
	Parent       int
	Nodes        []string
	Direction    Direction
	HasDirection bool
}

// Node 按 ID 查找节点，找不到返回 nil。
func (g *Graph) Node(id string) *Node {
	for _, n := range g.Nodes {
		if n.ID == id {
			return n
		}
	}
	return nil
}

// flowLike 报告图表是否由通用的分层引擎布局。
func (g *Graph) flowLike() bool {
	switch g.Kind {
	case KindSequence, KindGantt, KindPie:
		return false
	}
	return true
}
