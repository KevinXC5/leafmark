package diagram

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

func attributes(s string) (Attributes, error) {
	a := Attributes{}
	for _, part := range strings.Split(s, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), ":")
		if !ok {
			return a, fmt.Errorf("无效的样式 %q", clip(part))
		}
		value = strings.TrimSpace(value)
		switch key {
		case "fill", "stroke", "color":
			if !safeColor(value) {
				return a, fmt.Errorf("无效的颜色 %q", clip(value))
			}
			switch key {
			case "fill":
				a.Fill = value
			case "stroke":
				a.Stroke = value
			case "color":
				a.Color = value
			}
		case "stroke-width":
			v, err := strconv.ParseFloat(strings.TrimSuffix(value, "px"), 32)
			if err != nil || v <= 0 || v > 20 {
				return a, fmt.Errorf("无效的线宽")
			}
			a.StrokeWidth = float32(v)
		case "stroke-dasharray":
			a.Dashed = value != "0"
		default:
			return a, fmt.Errorf("不支持的样式属性 %q", key)
		}
	}
	return a, nil
}
func safeColor(s string) bool {
	if s == "none" || s == "transparent" {
		return true
	}
	if strings.HasPrefix(s, "#") && (len(s) == 4 || len(s) == 7 || len(s) == 9) {
		_, err := strconv.ParseUint(s[1:], 16, 32)
		return err == nil
	}
	switch s {
	case "red", "green", "blue", "black", "white", "gray", "orange", "yellow", "purple", "pink", "lightblue", "lightgreen":
		return true
	}
	return false
}

// cssColor 把安全颜色子集转成绘制颜色。none 与 transparent 的透明度为 0。
func cssColor(s string) (ui.Color, bool) {
	switch s {
	case "", "none", "transparent":
		return ui.Color{}, s != ""
	case "red":
		return ui.Hex("#d03b3b"), true
	case "green":
		return ui.Hex("#1b8a4a"), true
	case "blue":
		return ui.Hex("#2a78d6"), true
	case "black":
		return ui.Hex("#1c1a17"), true
	case "white":
		return ui.Hex("#ffffff"), true
	case "gray":
		return ui.Hex("#8a8178"), true
	case "orange":
		return ui.Hex("#eb6834"), true
	case "yellow":
		return ui.Hex("#eda100"), true
	case "purple":
		return ui.Hex("#6b4ca8"), true
	case "pink":
		return ui.Hex("#e87ba4"), true
	case "lightblue":
		return ui.Hex("#b9d6f2"), true
	case "lightgreen":
		return ui.Hex("#c6e6d4"), true
	}
	if strings.HasPrefix(s, "#") && (len(s) == 4 || len(s) == 7 || len(s) == 9) {
		if _, err := strconv.ParseUint(s[1:], 16, 32); err == nil {
			return ui.Hex(s), true
		}
	}
	return ui.Color{}, false
}

func mergeAttributes(a, b Attributes) Attributes {
	if b.Fill != "" {
		a.Fill = b.Fill
	}
	if b.Stroke != "" {
		a.Stroke = b.Stroke
	}
	if b.Color != "" {
		a.Color = b.Color
	}
	if b.StrokeWidth > 0 {
		a.StrokeWidth = b.StrokeWidth
	}
	a.Dashed = a.Dashed || b.Dashed
	return a
}
func (p *parser) styleStatement(word, rest string) error {
	ids, spec := cutWord(rest)
	switch word {
	case "classDef":
		a, err := attributes(spec)
		if err != nil {
			return err
		}
		for _, id := range strings.Split(ids, ",") {
			p.g.classes[id] = a
		}
	case "class":
		if spec == "" {
			return fmt.Errorf("class 缺少类名")
		}
		for _, id := range strings.Split(ids, ",") {
			n := p.g.Nodes[p.node(id)]
			n.Classes = append(n.Classes, strings.Split(spec, ",")...)
		}
	case "style":
		a, err := attributes(spec)
		if err != nil {
			return err
		}
		for _, id := range strings.Split(ids, ",") {
			n := p.g.Nodes[p.node(id)]
			n.Style = mergeAttributes(n.Style, a)
		}
	case "linkStyle":
		a, err := attributes(spec)
		if err != nil {
			return err
		}
		if ids == "default" {
			for _, e := range p.g.Edges {
				e.Style = mergeAttributes(e.Style, a)
			}
		} else {
			for _, id := range strings.Split(ids, ",") {
				i, err := strconv.Atoi(id)
				if err != nil || i < 0 || i >= len(p.g.Edges) {
					return fmt.Errorf("linkStyle 的连线下标无效")
				}
				p.g.Edges[i].Style = mergeAttributes(p.g.Edges[i].Style, a)
			}
		}
	case "click":
		spec = strings.TrimPrefix(spec, "href ")
		sc := strings.TrimSpace(spec)
		if !strings.HasPrefix(sc, "\"") {
			return fmt.Errorf("click 仅支持 URL，不执行回调")
		}
		end := strings.Index(sc[1:], "\"")
		if end < 0 {
			return fmt.Errorf("click URL 未闭合")
		}
		target := sc[1 : end+1]
		u, err := url.Parse(target)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "mailto") {
			return fmt.Errorf("click 仅支持 http、https 或 mailto")
		}
		n := p.g.Nodes[p.node(ids)]
		n.URL = target
		tail := strings.TrimSpace(sc[end+2:])
		if strings.HasPrefix(tail, "\"") {
			if i := strings.Index(tail[1:], "\""); i >= 0 {
				n.Tooltip = cleanText(tail[1 : i+1])
			}
		}
	}
	return nil
}
func (g *Graph) nodeAttributes(n *Node) Attributes {
	a := g.classes["default"]
	for _, c := range n.Classes {
		a = mergeAttributes(a, g.classes[c])
	}
	return mergeAttributes(a, n.Style)
}

// Hit 是节点点击结果，URL 只包含解析时验证过的公开链接协议。
type Hit struct{ ID, URL, Tooltip string }

// HitTest 的坐标相对于图左上角，使用布局后的实际尺寸。
func (l *Layout) HitTest(x, y float32) (Hit, bool) {
	for i := len(l.geo.nodes) - 1; i >= 0; i-- {
		n := l.geo.nodes[i]
		if x >= n.x-n.w/2 && x <= n.x+n.w/2 && y >= n.y-n.h/2 && y <= n.y+n.h/2 && n.node.URL != "" {
			return Hit{n.node.ID, n.node.URL, n.node.Tooltip}, true
		}
	}
	return Hit{}, false
}
