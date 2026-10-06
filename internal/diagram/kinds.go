package diagram

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Attributes 是安全的 Mermaid 样式子集，不执行 CSS 或脚本。
type Attributes struct {
	Fill, Stroke, Color string
	StrokeWidth         float32
	Dashed              bool
}

// sequenceBox 是时序图里的分组框。定义由 diagram 的实现补全，这里只占位让包可编译。
type sequenceBox struct{}

type sequenceEvent struct {
	kind, from, to, text string
	dashed               bool
	open                 bool // >> 与 ) 是开口箭头
}
type ganttTask struct {
	id, text, section, status string
	start, end                time.Time
}
type pieSlice struct {
	text  string
	value float64
}

func parseOther(kind string, stmts []statement) (*Graph, error) {
	g := &Graph{Kind: kind, classes: map[string]Attributes{}}
	p := &parser{g: g, index: map[string]int{}}
	add := func(id, text string) *Node {
		n := g.Nodes[p.node(id)]
		if text != "" {
			n.Text = cleanText(text)
		}
		return n
	}
	edge := func(a, b, label, relation string, dashed bool) {
		ai, bi := p.node(a), p.node(b)
		e := &Edge{from: ai, to: bi, Label: cleanText(label), Head: ArrowNormal, Length: 1, Relation: relation}
		if dashed {
			e.Line = LineDotted
		}
		switch relation {
		case "<|--", "--|>":
			e.Head = ArrowTriangle
		case "*--", "--*":
			e.Head = ArrowDiamondFilled
		case "o--", "--o":
			e.Head = ArrowDiamond
		case "<..", "..>":
			e.Head = ArrowTriangle
			e.Line = LineDotted
		case "<--", "-->":
			e.Head = ArrowOpen
		case "..":
			e.Head = ArrowOpen
			e.Line = LineDotted
		}
		// 标记在左侧时箭头指向起点，布局上调换两端，绘制仍从 From 指向 To。
		if relation == "<|--" || relation == "*--" || relation == "o--" || relation == "<.." || relation == "<--" {
			e.from, e.to = bi, ai
		}
		g.Edges = append(g.Edges, e)
	}
	block := ""
	section := ""
	dateFormat := "2006-01-02"
	var previous time.Time
	tasks := map[string]ganttTask{}
	frames := 0
	for _, st := range stmts {
		s := strings.TrimSpace(st.text)
		if s == "" {
			continue
		}
		word, rest := cutWord(s)
		fail := func() (*Graph, error) {
			return nil, fmt.Errorf("diagram: 第 %d 行: 不支持或无效的 %s 语句 %q", st.line, kind, clip(s))
		}
		if len(g.Nodes) > maxNodes || len(g.Edges) > maxEdges || len(g.sequence) > maxEdges || len(g.tasks) > maxNodes || len(g.slices) > maxNodes {
			return nil, fmt.Errorf("diagram: 图表规模超过上限")
		}
		if word == "title" {
			g.Title = cleanText(rest)
			continue
		}
		switch kind {
		case "pie":
			m := regexp.MustCompile(`^"(.*)"\s*:\s*([0-9]+(?:\.[0-9]+)?)$`).FindStringSubmatch(s)
			if m == nil {
				return fail()
			}
			v, _ := strconv.ParseFloat(m[2], 64)
			if !isFinite(v) {
				return fail()
			}
			g.slices = append(g.slices, pieSlice{cleanText(m[1]), v})
		case "sequenceDiagram":
			if word == "autonumber" {
				if rest != "" {
					return fail()
				}
				g.sequence = append(g.sequence, sequenceEvent{kind: "autonumber"})
				continue
			}
			if word == "participant" || word == "actor" {
				id, label, _ := strings.Cut(rest, " as ")
				id = strings.TrimSpace(id)
				if id == "" {
					return fail()
				}
				add(id, label)
				continue
			}
			if word == "activate" || word == "deactivate" {
				if rest == "" {
					return fail()
				}
				add(rest, "")
				g.sequence = append(g.sequence, sequenceEvent{kind: word, from: rest})
				continue
			}
			if word == "Note" || word == "note" {
				pos, text, ok := strings.Cut(rest, ":")
				if !ok {
					return fail()
				}
				where, ids := cutWord(pos)
				if where == "left" || where == "right" {
					ids = strings.TrimPrefix(ids, "of ")
				}
				actors := strings.Split(ids, ",")
				for _, id := range actors {
					add(strings.TrimSpace(id), "")
				}
				to := strings.TrimSpace(actors[len(actors)-1])
				g.sequence = append(g.sequence, sequenceEvent{kind: "note " + where, from: strings.TrimSpace(actors[0]), to: to, text: cleanText(text)})
				continue
			}
			switch word {
			case "loop", "alt", "opt", "par", "critical", "break", "rect":
				frames++
				g.sequence = append(g.sequence, sequenceEvent{kind: "frame", text: word + " " + cleanText(rest)})
				continue
			case "else", "and", "option":
				if frames == 0 {
					return fail()
				}
				g.sequence = append(g.sequence, sequenceEvent{kind: "divider", text: cleanText(rest)})
				continue
			case "end":
				if frames == 0 {
					return fail()
				}
				frames--
				g.sequence = append(g.sequence, sequenceEvent{kind: "end"})
				continue
			}
			m := regexp.MustCompile(`^([^\s:]+?)\s*(-->>|->>|-->|->|--x|-x|--\)|-\))\s*([+-]?)([^\s:]+)\s*:\s*(.*)$`).FindStringSubmatch(s)
			if m == nil {
				return fail()
			}
			add(m[1], "")
			add(m[4], "")
			g.sequence = append(g.sequence, sequenceEvent{kind: "message", from: m[1], to: m[4], text: cleanText(m[5]), dashed: strings.HasPrefix(m[2], "--"), open: strings.Contains(m[2], ">>") || strings.Contains(m[2], ")")})
			if m[3] == "+" {
				g.sequence = append(g.sequence, sequenceEvent{kind: "activate", from: m[4]})
			}
			if m[3] == "-" {
				g.sequence = append(g.sequence, sequenceEvent{kind: "deactivate", from: m[4]})
			}
		case "gantt":
			switch word {
			case "dateFormat":
				dateFormat = strings.NewReplacer("YYYY", "2006", "MM", "01", "DD", "02", "HH", "15", "mm", "04", "ss", "05").Replace(rest)
				continue
			case "axisFormat":
				continue
			case "section":
				section = cleanText(rest)
				continue
			}
			label, spec, ok := strings.Cut(s, ":")
			if !ok {
				return fail()
			}
			parts := strings.Split(spec, ",")
			for i := range parts {
				parts[i] = strings.TrimSpace(parts[i])
			}
			status := ""
			for len(parts) > 0 && (parts[0] == "done" || parts[0] == "active" || parts[0] == "crit" || parts[0] == "milestone") {
				status += parts[0] + " "
				parts = parts[1:]
			}
			id := fmt.Sprintf("task%d", len(g.tasks))
			if len(parts) == 3 {
				id = parts[0]
				parts = parts[1:]
			}
			if len(parts) < 1 || len(parts) > 2 {
				return fail()
			}
			start := previous
			endSpec := parts[len(parts)-1]
			if len(parts) == 2 {
				if strings.HasPrefix(parts[0], "after ") {
					for _, dep := range strings.Fields(strings.TrimPrefix(parts[0], "after ")) {
						task, ok := tasks[dep]
						if !ok {
							return fail()
						}
						if task.end.After(start) {
							start = task.end
						}
					}
				} else {
					var err error
					start, err = time.Parse(dateFormat, parts[0])
					if err != nil {
						return fail()
					}
				}
			}
			if start.IsZero() {
				return fail()
			}
			end, err := time.Parse(dateFormat, endSpec)
			if err != nil {
				m := regexp.MustCompile(`^(\d+)([dhwm])$`).FindStringSubmatch(endSpec)
				if m == nil {
					return fail()
				}
				n, _ := strconv.Atoi(m[1])
				factor := time.Hour * 24
				switch m[2] {
				case "h":
					factor = time.Hour
				case "w":
					factor *= 7
				case "m":
					factor = time.Minute
				}
				if n > 100000 {
					return fail()
				}
				end = start.Add(time.Duration(n) * factor)
			}
			if end.Before(start) {
				return fail()
			}
			t := ganttTask{id, cleanText(label), section, strings.TrimSpace(status), start, end}
			g.tasks = append(g.tasks, t)
			tasks[id] = t
			previous = end
		case "classDiagram", "erDiagram":
			if word == "direction" {
				d, ok := parseDirection(rest)
				if !ok {
					return fail()
				}
				g.Direction = d
				continue
			}
			if block != "" {
				if s == "}" {
					block = ""
					continue
				}
				n := add(block, "")
				line := cleanText(s)
				n.Members = append(n.Members, line)
				if kind == "classDiagram" {
					if strings.Contains(line, "(") {
						n.methods = append(n.methods, line)
					} else {
						n.attrs = append(n.attrs, line)
					}
				} else {
					n.rows = append(n.rows, erRow(line))
				}
				continue
			}
			if strings.HasSuffix(s, "{") {
				id := strings.TrimSpace(strings.TrimSuffix(s, "{"))
				interfaceBlock := strings.HasPrefix(id, "interface ")
				id = strings.TrimPrefix(id, "class ")
				id = strings.TrimPrefix(id, "interface ")
				anno := ""
				if i := strings.Index(id, "<<"); i >= 0 {
					if j := strings.Index(id[i:], ">>"); j >= 0 {
						anno = cleanText(id[i+2 : i+j])
						id = strings.TrimSpace(id[:i] + id[i+j+2:])
					}
				}
				if id == "" {
					return fail()
				}
				n := add(id, "")
				if kind == "classDiagram" {
					n.Shape = ShapeClass
					if interfaceBlock && anno == "" {
						anno = "interface"
					}
					if anno != "" {
						n.annotation = anno
					}
				} else {
					n.Shape = ShapeEntity
				}
				block = id
				continue
			}
			if kind == "classDiagram" && (word == "class" || word == "interface") {
				id := strings.TrimSpace(rest)
				anno := ""
				if i := strings.Index(id, "<<"); i >= 0 {
					if j := strings.Index(id[i:], ">>"); j >= 0 {
						anno = cleanText(id[i+2 : i+j])
						id = strings.TrimSpace(id[:i] + id[i+j+2:])
					}
				}
				label := ""
				if a, b, ok := strings.Cut(id, "["); ok {
					id = strings.TrimSpace(a)
					label = strings.TrimSuffix(strings.TrimSpace(b), "]")
				}
				if id == "" {
					return fail()
				}
				n := add(id, label)
				n.Shape = ShapeClass
				if word == "interface" && anno == "" {
					anno = "interface"
				}
				if anno != "" {
					n.annotation = anno
				}
				continue
			}
			if kind == "classDiagram" {
				if id, member, ok := strings.Cut(s, ":"); ok && !strings.ContainsAny(id, "<>|.*-") {
					n := add(strings.TrimSpace(id), "")
					n.Shape = ShapeClass
					line := cleanText(member)
					n.Members = append(n.Members, line)
					if strings.Contains(line, "(") {
						n.methods = append(n.methods, line)
					} else {
						n.attrs = append(n.attrs, line)
					}
					continue
				}
			}
			pattern := `^(\S+?)\s*(?:"([^"]*)"\s*)?(<\|--|--\|>|\*--|--\*|o--|--o|<\.\.|\.\.>|<--|-->|--|\.\.)\s*(?:"([^"]*)"\s*)?(\S+?)(?:\s*:\s*(.*))?$`
			if kind == "erDiagram" {
				pattern = `^(\S+)\s+([|o}{]{2})(--|\.\.)([|o}{]{2})\s+(\S+)\s*:\s*(.*)$`
			}
			m := regexp.MustCompile(pattern).FindStringSubmatch(s)
			if m == nil {
				if !strings.ContainsAny(s, " \t:") {
					add(s, "")
					continue
				}
				return fail()
			}
			left, right := g.Nodes[p.node(m[1])], g.Nodes[p.node(m[5])]
			if kind == "classDiagram" {
				left.Shape, right.Shape = ShapeClass, ShapeClass
			} else {
				left.Shape, right.Shape = ShapeEntity, ShapeEntity
			}
			edge(m[1], m[5], m[6], m[3], strings.Contains(m[3], ".."))
			e := g.Edges[len(g.Edges)-1]
			if kind != "erDiagram" {
				e.CardinalityFrom, e.CardinalityTo = m[2], m[4]
			}
			if kind == "erDiagram" {
				e.Tail, e.Head = erMark(m[2]), erMark(m[4])
				if m[3] == ".." {
					e.Line = LineDotted
				}
				e.Relation = m[2] + m[3] + m[4]
			}
		case "stateDiagram", "stateDiagram-v2":
			if word == "direction" {
				d, ok := parseDirection(rest)
				if !ok {
					return fail()
				}
				g.Direction = d
				continue
			}
			if word == "state" {
				if strings.HasSuffix(rest, "{") {
					id := strings.TrimSpace(strings.TrimSuffix(rest, "{"))
					if strings.Contains(id, " as ") {
						label, alias, _ := strings.Cut(id, " as ")
						id = alias
						add(id, label)
					}
					if err := p.openSubgraph(id); err != nil {
						return nil, err
					}
					continue
				}
				label, id, ok := strings.Cut(rest, " as ")
				if ok {
					add(strings.TrimSpace(id), label)
				} else {
					add(rest, "")
				}
				continue
			}
			if s == "}" {
				if len(p.stack) == 0 {
					return fail()
				}
				p.stack = p.stack[:len(p.stack)-1]
				continue
			}
			if a, b, ok := strings.Cut(s, "-->"); ok {
				dest, label, _ := strings.Cut(b, ":")
				a = strings.TrimSpace(a)
				dest = strings.TrimSpace(dest)
				if a == "[*]" {
					a = fmt.Sprintf("__start%d", len(g.Nodes))
					n := add(a, "")
					n.Text, n.Shape = "", ShapeStart
				}
				if dest == "[*]" {
					dest = fmt.Sprintf("__end%d", len(g.Nodes))
					n := add(dest, "")
					n.Text, n.Shape = "", ShapeEnd
				}
				if n := g.Node(a); n != nil && n.Shape == ShapeRect {
					n.Shape = ShapeState
				}
				if n := g.Node(dest); n != nil && n.Shape == ShapeRect {
					n.Shape = ShapeState
				}
				edge(a, dest, label, "", false)
				continue
			}
			if id, text, ok := strings.Cut(s, ":"); ok {
				add(strings.TrimSpace(id), text)
				continue
			}
			return fail()
		}
	}
	if block != "" || frames != 0 {
		return nil, fmt.Errorf("diagram: 未闭合的图表块")
	}
	if kind == "pie" {
		sum := 0.
		for _, s := range g.slices {
			sum += s.value
		}
		if sum <= 0 || !isFinite(sum) {
			return nil, fmt.Errorf("diagram: 饼图总量必须大于零")
		}
	}
	if kind == "gantt" && len(g.tasks) == 0 {
		return nil, fmt.Errorf("diagram: 甘特图缺少任务")
	}
	switch kind {
	case "classDiagram":
		for _, n := range g.Nodes {
			if n.Shape == ShapeRect {
				n.Shape = ShapeClass
			}
		}
	case "erDiagram":
		for _, n := range g.Nodes {
			if n.Shape == ShapeRect {
				n.Shape = ShapeEntity
			}
		}
	case "stateDiagram", "stateDiagram-v2":
		for _, n := range g.Nodes {
			if n.Shape == ShapeRect {
				n.Shape = ShapeState
			}
		}
	}
	return p.finish()
}

func erRow(line string) []string {
	row := []string{"", "", "", ""}
	rest := line
	if i := strings.IndexByte(rest, '"'); i >= 0 {
		if j := strings.LastIndexByte(rest, '"'); j > i {
			row[3] = cleanText(rest[i+1 : j])
			rest = strings.TrimSpace(rest[:i])
		}
	}
	fields := strings.Fields(rest)
	if len(fields) > 0 {
		row[0] = fields[0]
	}
	if len(fields) > 1 {
		row[1] = fields[1]
	}
	if len(fields) > 2 {
		row[2] = strings.Join(fields[2:], " ")
	}
	return row
}

func erMark(mark string) Arrow {
	switch mark {
	case "||":
		return ArrowOne
	case "|o", "o|":
		return ArrowZeroOrOne
	case "}|", "|{":
		return ArrowOneOrMany
	case "}o", "o{":
		return ArrowZeroOrMany
	}
	return ArrowNone
}

func isFinite(v float64) bool { return !math.IsInf(v, 0) && !math.IsNaN(v) }
