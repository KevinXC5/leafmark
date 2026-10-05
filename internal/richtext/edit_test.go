package richtext

import "testing"

func TestInsertInherit(t *testing.T) {
	d := Parse("**bold**\n")
	sel := d.Insert(4, "X")
	if sel.Start != 5 {
		t.Fatalf("caret %v", sel)
	}
	b := d.Blocks()[0]
	if len(b.Runs) != 1 || b.Runs[0].Text != "boldX" || b.Runs[0].Marks != MarkBold {
		t.Fatalf("runs %+v", b.Runs)
	}
	md := d.Markdown()
	if md != "**boldX**" && md != "**boldX**\n" {
		t.Fatalf("md %q", md)
	}
}

func TestPendingMark(t *testing.T) {
	d := Parse("ab\n")
	d.ToggleMark(Selection{1, 1}, MarkItalic)
	d.Insert(1, "Z")
	b := d.Blocks()[0]
	found := false
	for _, r := range b.Runs {
		if r.Text == "Z" && r.Marks&MarkItalic != 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("runs %+v md %q", b.Runs, d.Markdown())
	}
}

func TestDeleteAcross(t *testing.T) {
	d := Parse("abcd\n\nEFGH\n")
	// text: abcd \n EFGH
	if d.Text() != "abcd\nEFGH" {
		t.Fatalf("text %q", d.Text())
	}
	d.Delete(2, 7) // cd \n EF
	if d.Text() != "abGH" {
		t.Fatalf("text %q md %q", d.Text(), d.Markdown())
	}
}

func TestUndoRedo(t *testing.T) {
	d := Parse("ab\n")
	d.Insert(1, "X")
	d.Insert(2, "Y")
	if _, ok := d.Undo(); !ok {
		t.Fatal("undo")
	}
	if d.Text() != "ab" {
		t.Fatalf("after undo %q", d.Text())
	}
	if _, ok := d.Redo(); !ok {
		t.Fatal("redo")
	}
	if d.Text() != "aXYb" {
		t.Fatalf("after redo %q", d.Text())
	}
}

func TestRawReject(t *testing.T) {
	d := Parse("<div>\nx\n</div>\n")
	before := d.Markdown()
	d.Insert(0, "Z")
	if d.Markdown() != before {
		t.Fatalf("raw changed %q", d.Markdown())
	}
	if err := d.ReplaceRaw(0, "hello"); err != nil {
		t.Fatal(err)
	}
	if d.Text() != "hello" {
		t.Fatalf("replaced %q", d.Text())
	}
}

func TestImage(t *testing.T) {
	d := Parse("hello\n")
	d.InsertImage(5, "a", "./a.png")
	md := d.Markdown()
	if !contains(md, "![a](./a.png)") {
		t.Fatalf("md %q", md)
	}
	found := false
	for _, b := range d.Blocks() {
		if b.Kind == Image && b.Alt == "a" && b.URL == "./a.png" {
			found = true
		}
	}
	if !found {
		t.Fatalf("blocks %+v", d.Blocks())
	}
}

func TestEscape(t *testing.T) {
	d := Parse("hello\n")
	d.Insert(5, "*")
	md := d.Markdown()
	again := Parse(md)
	if again.Text() != "hello*" {
		t.Fatalf("md %q text %q", md, again.Text())
	}
}

func TestTableEdit(t *testing.T) {
	d := Parse("| a | b |\n| --- | --- |\n| 1 | 2 |\n")
	if d.Text() != "a\tb\n1\t2" {
		t.Fatalf("text %q", d.Text())
	}
	d.Insert(1, "X")
	if d.Text() != "aX\tb\n1\t2" {
		t.Fatalf("text %q", d.Text())
	}
}

func TestCode(t *testing.T) {
	d := Parse("```go\nfmt\n```\n")
	if d.Blocks()[0].Kind != Code || d.Blocks()[0].Lang != "go" {
		t.Fatalf("%+v", d.Blocks()[0])
	}
	d.Insert(3, "!")
	if d.Blocks()[0].Code != "fmt!\n" && d.Blocks()[0].Code != "fmt!" {
		t.Fatalf("code %q", d.Blocks()[0].Code)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || (func() bool { return len([]rune(s)) > 0 && (index(s, sub) >= 0) })())
}
func index(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
