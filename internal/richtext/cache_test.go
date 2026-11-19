package richtext

import "testing"

func TestMarkdownCacheAndRevision(t *testing.T) {
	doc := Parse("# 标题\n\n- [ ] 待办\n\n- 列表\n")
	if doc.Revision() == 0 {
		t.Fatal("解析后版本号应大于 0")
	}
	assertFresh := func(step string, prevRev uint64) uint64 {
		t.Helper()
		rev := doc.Revision()
		if rev <= prevRev {
			t.Fatalf("%s 后版本号未递增：%d → %d", step, prevRev, rev)
		}
		got := doc.Markdown()
		if !doc.mdValid {
			t.Fatalf("%s 后未写入 Markdown 缓存", step)
		}
		// 强制失效后再序列化一次，确认缓存内容与现算完全一致。
		doc.mdValid = false
		want := doc.Markdown()
		if got != want {
			t.Fatalf("%s 缓存不一致：\ngot:  %q\nwant: %q", step, got, want)
		}
		return rev
	}

	rev := doc.Revision()
	if first, second := doc.Markdown(), doc.Markdown(); first != second || !doc.mdValid {
		t.Fatalf("重复读取 Markdown 应命中缓存：%q vs %q", first, second)
	}

	doc.Insert(2, "追加")
	rev = assertFresh("Insert", rev)

	doc.ToggleMark(Selection{Start: 0, End: 2}, MarkBold)
	rev = assertFresh("ToggleMark", rev)

	doc.SetChecked(1, true)
	rev = assertFresh("SetChecked", rev)

	doc.SetOrdered(2, true)
	rev = assertFresh("SetOrdered", rev)

	if _, ok := doc.Undo(); !ok {
		t.Fatal("Undo 应成功")
	}
	rev = assertFresh("Undo", rev)

	if _, ok := doc.Redo(); !ok {
		t.Fatal("Redo 应成功")
	}
	assertFresh("Redo", rev)

	plain := NewPlainText("纯文本")
	if plain.Markdown() != "纯文本" || !plain.mdValid {
		t.Fatalf("纯文本缓存异常：%q", plain.Markdown())
	}
	prevPlain := plain.Revision()
	plain.Insert(3, "！")
	if plain.Revision() <= prevPlain || plain.Markdown() != "纯文本！" {
		t.Fatalf("纯文本编辑后缓存未刷新：%d → %d，%q", prevPlain, plain.Revision(), plain.Markdown())
	}
}

func TestInlineMathOrigin(t *testing.T) {
	doc := Parse("前 $x+y$ 后")
	// 纯文本为 "前 x+y 后"，其中 x+y 位于偏移 [2, 5)。
	if got := doc.InlineMathOrigin(0); got != -1 {
		t.Fatalf("普通文字偏移 0 应返回 -1，实际 %d", got)
	}
	if got := doc.InlineMathOrigin(2); got != -1 {
		t.Fatalf("公式起点不属于严格内部，应返回 -1，实际 %d", got)
	}
	for _, inside := range []int{3, 4} {
		if got := doc.InlineMathOrigin(inside); got != 2 {
			t.Fatalf("公式内部偏移 %d 应返回起点 2，实际 %d", inside, got)
		}
	}
	if got := doc.InlineMathOrigin(5); got != -1 {
		t.Fatalf("公式终点不属于严格内部，应返回 -1，实际 %d", got)
	}
}
