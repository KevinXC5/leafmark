package diagram

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
	ArrowNone   Arrow = iota
	ArrowNormal       // >
	ArrowCircle       // o
	ArrowCross        // x
)

// Graph 是解析后的流程图。
type Graph struct {
	Direction Direction
	Nodes     []*Node
	Edges     []*Edge
	Subgraphs []*Subgraph
}

// Node 是一个节点。Text 可含换行，未定义文字时等于 ID。
type Node struct {
	ID    string
	Text  string
	Shape Shape

	group int // 所属子图在 Graph.Subgraphs 中的下标，-1 表示不属于任何子图
}

// Edge 是一条连线。Length 是它至少跨过的层数，--> 为 1，---> 为 2。
type Edge struct {
	From, To   string
	Label      string
	Line       LineStyle
	Head, Tail Arrow
	Length     int

	from, to int // 端点在 Graph.Nodes 中的下标
}

// Subgraph 是一个子图。Parent 是外层子图的下标，-1 表示顶层；
// Nodes 只列出直接属于它的节点。
type Subgraph struct {
	ID     string
	Title  string
	Parent int
	Nodes  []string
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
