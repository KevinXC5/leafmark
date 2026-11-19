package richtext

import (
	"fmt"
	"strings"
	"testing"
)

func TestEditableSpanBlocksAndInlines(t *testing.T) {
	t.Run("普通文本与越界区间", func(t *testing.T) {
		doc := Parse("甲**乙**[链接](https://example.com)\n\n第二段\n")
		n := doc.Len()
		if !doc.EditableSpan(0, n) {
			t.Fatalf("普通段落整段应可编辑，text=%q", doc.Text())
		}
		if !doc.EditableSpan(0, 0) || !doc.EditableSpan(n, n) || !doc.EditableSpan(2, 2) {
			t.Fatal("合法范围内的空区间应返回 true")
		}
		for _, tc := range []struct {
			name       string
			start, end int
		}{
			{"起点为负", -1, 1},
			{"终点小于起点", 3, 2},
			{"终点越界", 0, n + 1},
			{"起止均越界", n + 1, n + 2},
		} {
			if doc.EditableSpan(tc.start, tc.end) {
				t.Fatalf("%s (%d, %d) 应返回 false", tc.name, tc.start, tc.end)
			}
		}
	})

	t.Run("代码块", func(t *testing.T) {
		doc := Parse("```go\nfmt.Println(1)\n```\n")
		if got := doc.Blocks(); len(got) != 1 || got[0].Kind != Code {
			t.Fatalf("预期解析为代码块，实际 %+v", got)
		}
		if !doc.EditableSpan(0, doc.Len()) || !doc.EditableSpan(4, 11) {
			t.Fatalf("代码块内容应可编辑，text=%q", doc.Text())
		}
	})

	t.Run("表格单元格", func(t *testing.T) {
		doc := Parse("| 列一 | 列二 |\n| --- | --- |\n| 值一 | 值二 |\n")
		if got := doc.Blocks(); len(got) != 1 || got[0].Kind != TableBlock {
			t.Fatalf("预期解析为表格块，实际 %+v", got)
		}
		// 单元格内部以及跨列、跨行区间在 EditableSpan 中均视为可直接编辑文本。
		if !doc.EditableSpan(0, 2) || !doc.EditableSpan(3, 5) || !doc.EditableSpan(0, doc.Len()) {
			t.Fatalf("表格单元格与整表区间应可编辑，text=%q", doc.Text())
		}
	})

	t.Run("含ReferenceDef文档", func(t *testing.T) {
		doc := Parse("[叶脉][ref]\n\n[ref]: https://example.com \"示例\"\n\n尾段\n")
		hasRef := false
		for _, b := range doc.Blocks() {
			if b.Kind == ReferenceDef {
				hasRef = true
			}
		}
		if !hasRef {
			t.Fatalf("预期包含 ReferenceDef 块，实际 %+v", doc.Blocks())
		}
		// ReferenceDef 不占可见正文坐标，前后段落均可编辑。
		if doc.Text() != "叶脉\n尾段" {
			t.Fatalf("可见正文不符合预期：%q", doc.Text())
		}
		if !doc.EditableSpan(0, doc.Len()) || !doc.EditableSpan(0, 2) || !doc.EditableSpan(3, 5) {
			t.Fatalf("含 ReferenceDef 的文档正文应可编辑")
		}
		if doc.EditableSpan(0, doc.Len()+1) {
			t.Fatal("越界区间应返回 false")
		}

		onlyRef := Parse("[ref]: https://example.com\n")
		if onlyRef.Len() != 0 {
			t.Fatalf("仅含 ReferenceDef 的文档可见正文应为空，实际 %q", onlyRef.Text())
		}
		if !onlyRef.EditableSpan(0, 0) || onlyRef.EditableSpan(0, 1) {
			t.Fatal("仅含 ReferenceDef 的文档边界判断错误")
		}
	})

	t.Run("行内公式MarkMath", func(t *testing.T) {
		doc := Parse("前 $x+y$ 后\n")
		// 可见正文为 "前 x+y 后"，其中公式 "x+y" 位于 rune 区间 [2, 5)。
		if doc.Text() != "前 x+y 后" {
			t.Fatalf("可见正文不符合预期：%q", doc.Text())
		}
		if !doc.EditableSpan(0, 2) || !doc.EditableSpan(5, 7) {
			t.Fatal("公式前后的普通文字应可编辑")
		}
		for _, span := range [][2]int{{2, 5}, {2, 3}, {3, 4}, {1, 3}, {4, 6}, {0, 7}} {
			if doc.EditableSpan(span[0], span[1]) {
				t.Fatalf("包含行内公式的区间 [%d, %d) 应不可编辑", span[0], span[1])
			}
		}
	})

	t.Run("脚注引用", func(t *testing.T) {
		doc := Parse("正文[^n1]结尾\n\n[^n1]: 注释正文\n")
		// 可见正文为 "正文n1结尾\n注释正文"，其中脚注引用 "n1" 位于 [2, 4)。
		if doc.Text() != "正文n1结尾\n注释正文" {
			t.Fatalf("可见正文不符合预期：%q", doc.Text())
		}
		if !doc.EditableSpan(0, 2) || !doc.EditableSpan(4, 6) || !doc.EditableSpan(7, 11) {
			t.Fatal("脚注引用前后文字与脚注定义正文应可编辑")
		}
		for _, span := range [][2]int{{2, 4}, {2, 3}, {1, 3}, {3, 5}, {0, 6}} {
			if doc.EditableSpan(span[0], span[1]) {
				t.Fatalf("包含脚注引用的区间 [%d, %d) 应不可编辑", span[0], span[1])
			}
		}
	})

	t.Run("行内图片", func(t *testing.T) {
		doc := Parse("图 ![alt](./a.png) 文\n")
		// 行内图片以单个 objectReplacement 占位，位于索引 2。
		want := "图 " + objectReplacement + " 文"
		if doc.Text() != want {
			t.Fatalf("可见正文不符合预期：%q，预期 %q", doc.Text(), want)
		}
		if !doc.EditableSpan(0, 2) || !doc.EditableSpan(3, 5) {
			t.Fatal("行内图片前后文字应可编辑")
		}
		for _, span := range [][2]int{{2, 3}, {1, 3}, {2, 4}, {0, 5}} {
			if doc.EditableSpan(span[0], span[1]) {
				t.Fatalf("包含行内图片的区间 [%d, %d) 应不可编辑", span[0], span[1])
			}
		}
	})

	t.Run("Raw与块级Image与Horizontal块", func(t *testing.T) {
		cases := []struct {
			name string
			md   string
			kind Kind
		}{
			{"Raw块", "前段\n\n<script>\nx\n</script>\n\n后段\n", Raw},
			{"块级Image", "前段\n\n![说明](./pic.png)\n\n后段\n", Image},
			{"Horizontal分隔线", "前段\n\n---\n\n后段\n", Horizontal},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				doc := Parse(tc.md)
				blocks := doc.Blocks()
				if len(blocks) != 3 || blocks[1].Kind != tc.kind {
					t.Fatalf("块结构不符合预期：%+v", blocks)
				}
				// 可见正文为 "前段\n￼\n后段"：前段 [0,2)，占位 [3,4)，后段 [5,7)。
				if !doc.EditableSpan(0, 2) || !doc.EditableSpan(5, 7) {
					t.Fatalf("相邻普通段落应可编辑，text=%q", doc.Text())
				}
				for _, span := range [][2]int{{3, 4}, {2, 4}, {3, 5}, {0, 7}} {
					if doc.EditableSpan(span[0], span[1]) {
						t.Fatalf("包含 %s 的区间 [%d, %d) 应不可编辑", tc.name, span[0], span[1])
					}
				}
			})
		}
	})
}

func TestUndoBudgetTrimKeepsMinStepsAndRestoresCorrectly(t *testing.T) {
	t.Run("超大文档裁剪至minUndoSteps且每步撤销正确", func(t *testing.T) {
		// 单步约 600K runes，16 步即约 9.6M > maxUndoRunes(8M)，会触发预算裁剪并保底保留 minUndoSteps。
		const baseLen = 600_000
		const totalEdits = minUndoSteps + 8
		base := strings.Repeat("a", baseLen)
		doc := NewPlainText(base)

		// 记录每步编辑前的预期前缀（每次都在偏移 0 插入独立标记，避免同点连续插入合并）。
		beforePrefixes := make([]string, 0, totalEdits)
		curPrefix := ""
		for i := 0; i < totalEdits; i++ {
			beforePrefixes = append(beforePrefixes, curPrefix)
			token := fmt.Sprintf("[%02d]", i)
			doc.Insert(0, token)
			curPrefix = token + curPrefix
		}

		if len(doc.undo) != minUndoSteps {
			t.Fatalf("撤销栈应保底保留 %d 步，实际 %d 步", minUndoSteps, len(doc.undo))
		}

		// 依次撤销保留的 minUndoSteps 步，验证每步文档长度、前缀及选区均正确。
		for step := 0; step < minUndoSteps; step++ {
			wantPrefix := beforePrefixes[totalEdits-1-step]
			sel, ok := doc.Undo()
			if !ok {
				t.Fatalf("第 %d 次撤销失败", step+1)
			}
			if sel != (Selection{Start: 0, End: 0}) {
				t.Fatalf("第 %d 次撤销选区不符合预期：%v", step+1, sel)
			}
			wantLen := baseLen + len(wantPrefix)
			if doc.Len() != wantLen {
				t.Fatalf("第 %d 次撤销长度不匹配：got %d, want %d", step+1, doc.Len(), wantLen)
			}
			gotHead := string(doc.text[:len(wantPrefix)+4])
			wantHead := wantPrefix + "aaaa"
			if gotHead != wantHead {
				t.Fatalf("第 %d 次撤销内容不匹配：got %q, want %q", step+1, gotHead, wantHead)
			}
		}

		if _, ok := doc.Undo(); ok {
			t.Fatal("已裁剪的更早历史不应继续撤销")
		}
	})

	t.Run("中等大文档按总rune预算动态裁剪", func(t *testing.T) {
		// 单步约 200K runes，50 步约 10M > maxUndoRunes(8M)，裁剪后应保留多于 minUndoSteps 步且总预算不超限。
		const baseLen = 200_000
		const totalEdits = 50
		base := strings.Repeat("b", baseLen)
		doc := NewPlainText(base)

		beforePrefixes := make([]string, 0, totalEdits)
		curPrefix := ""
		for i := 0; i < totalEdits; i++ {
			beforePrefixes = append(beforePrefixes, curPrefix)
			token := fmt.Sprintf("<%02d>", i)
			doc.Insert(0, token)
			curPrefix = token + curPrefix
		}

		kept := len(doc.undo)
		if kept <= minUndoSteps || kept >= totalEdits {
			t.Fatalf("预期按预算裁剪后保留步数位于 (%d, %d) 之间，实际 %d", minUndoSteps, totalEdits, kept)
		}
		totalRunes := 0
		for _, snap := range doc.undo {
			totalRunes += snap.runes
		}
		if totalRunes > maxUndoRunes {
			t.Fatalf("裁剪后总 rune 数 %d 超过预算 %d", totalRunes, maxUndoRunes)
		}

		for step := 0; step < kept; step++ {
			wantPrefix := beforePrefixes[totalEdits-1-step]
			if _, ok := doc.Undo(); !ok {
				t.Fatalf("第 %d 次撤销失败", step+1)
			}
			if doc.Len() != baseLen+len(wantPrefix) {
				t.Fatalf("第 %d 次撤销长度不匹配：got %d, want %d", step+1, doc.Len(), baseLen+len(wantPrefix))
			}
			if got := string(doc.text[:len(wantPrefix)+2]); got != wantPrefix+"bb" {
				t.Fatalf("第 %d 次撤销内容不匹配：got %q, want %q", step+1, got, wantPrefix+"bb")
			}
		}
		if _, ok := doc.Undo(); ok {
			t.Fatal("撤销耗尽后不应继续成功")
		}
	})
}
