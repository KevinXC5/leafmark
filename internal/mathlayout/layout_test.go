package mathlayout

import (
	"math/rand"
	"os"
	"regexp"
	"strings"
	"testing"
)

func mustLayout(t *testing.T, src string, display bool) *Box {
	t.Helper()
	b, err := Layout(src, 16, display)
	if err != nil {
		t.Fatalf("%q 排版失败：%v", src, err)
	}
	return b
}

func TestLayoutRelativeSizes(t *testing.T) {
	x := mustLayout(t, "x", false)
	if x.Width <= 0 || x.Ascent <= 0 {
		t.Fatalf("单字符盒子为空：%+v", x)
	}
	frac := mustLayout(t, `\frac{a}{b}`, true)
	if frac.Ascent+frac.Descent <= x.Ascent+x.Descent {
		t.Fatal("分式应高于单字符")
	}
	if sup := mustLayout(t, "x^2", false); sup.Ascent <= x.Ascent || sup.Width <= x.Width {
		t.Fatal("上标应抬高并加宽盒子")
	}
	if sub := mustLayout(t, "x_i", false); sub.Descent <= x.Descent {
		t.Fatal("下标应加深盒子")
	}
	inline, block := mustLayout(t, `\sum_{i=1}^{n} i`, false), mustLayout(t, `\sum_{i=1}^{n} i`, true)
	if block.Ascent+block.Descent <= inline.Ascent+inline.Descent || block.Width >= inline.Width+16 {
		t.Fatalf("块级求和的上下限应在正上正下：行内 %+v 块级 %+v", inline, block)
	}
	body := mustLayout(t, `\frac{\frac{1}{2}}{\frac{3}{4}}`, true)
	fenced := mustLayout(t, `\left(\frac{\frac{1}{2}}{\frac{3}{4}}\right)`, true)
	if fenced.Ascent+fenced.Descent < (body.Ascent+body.Descent)*.9 || fenced.Width <= body.Width {
		t.Fatal("可伸缩括号应随内容伸高")
	}
	if big := mustLayout(t, "x", false); mustLayoutSize(t, "x", 32).Width <= big.Width {
		t.Fatal("字号应放大盒子")
	}
}

func mustLayoutSize(t *testing.T, src string, size float32) *Box {
	t.Helper()
	b, err := Layout(src, size, false)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestLayoutRejectsMalformedInput(t *testing.T) {
	for _, src := range []string{`\frac{a}{b`, `\unknowncommand x`, `\left( x`, `\begin{pmatrix} a`, `x^`, `}`, `\begin{nosuchenv} a \end{nosuchenv}`} {
		if b, err := Layout(src, 16, true); err == nil {
			t.Fatalf("%q 应返回错误，得到 %+v", src, b)
		}
	}
}

func TestLayoutNeverPanics(t *testing.T) {
	pieces := []string{`\frac`, `\sqrt`, `{`, `}`, `^`, `_`, `\left(`, `\right)`, `\sum`, `x`, `1`, `&`, `\\`, `\begin{cases}`, `\end{cases}`, `\text{`, `[`, `]`, ` `, `\alpha`, `\hat`, `\`, `$`, "中", `\begin{`, `\right`, `\left`, `\mathbb`}
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 4000; i++ {
		var src strings.Builder
		for n := rng.Intn(12); n >= 0; n-- {
			src.WriteString(pieces[rng.Intn(len(pieces))])
		}
		for _, display := range []bool{false, true} {
			if b, err := Layout(src.String(), 16, display); err == nil && b == nil {
				t.Fatalf("%q 没有错误却返回空盒子", src.String())
			}
		}
	}
	// 嵌套很深的输入也必须返回，而不是耗尽栈。
	deep := strings.Repeat(`\frac{`, 400) + "x" + strings.Repeat("}{y}", 400)
	_, _ = Layout(deep, 16, true)
}

func TestSampleDocumentFormulasLayOut(t *testing.T) {
	display := regexp.MustCompile(`(?s)\$\$(.+?)\$\$`)
	inline := regexp.MustCompile(`\$([^\s$][^$\n]*?)\$`)
	found := 0
	for _, name := range []string{"山中来信", "叶脉笔记"} {
		data, err := os.ReadFile("../../tests/fixtures/samples/" + name + ".md")
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		for _, m := range display.FindAllStringSubmatch(text, -1) {
			mustLayout(t, m[1], true)
			found++
		}
		for _, m := range inline.FindAllStringSubmatch(display.ReplaceAllString(text, ""), -1) {
			mustLayout(t, m[1], false)
			found++
		}
	}
	if found < 3 {
		t.Fatalf("示例文档里只找到 %d 条公式", found)
	}
}
