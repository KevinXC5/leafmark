package mathlayout

import (
	"encoding/xml"
	"math/rand"
	"strings"
	"testing"
	"unicode"
)

// extendedSamples 覆盖 \middle、附加环境、文字命令与补充的 TeX 命令。
var extendedSamples = []string{
	`\left\{ x \in \mathbb{R} \,\middle|\, \frac{x^2}{2} > 1 \right\}`,
	`\left( \frac{a}{b} \middle/ \frac{c}{d} \right)`,
	`\left\langle \psi \middle| \hat{H} \middle| \phi \right\rangle`,
	`\begin{dcases} \frac{1}{2} & x > 0 \\ 0 & \text{otherwise} \end{dcases}`,
	`\begin{rcases} a \\ b \end{rcases} \Rightarrow c`,
	`\begin{alignat}{2} a &= b & \quad c &= d \\ e &= f & g &= h \end{alignat}`,
	`\begin{alignedat}{2} a &= b \\ c &= d \end{alignedat}`,
	`\begin{multline} a + b + c \\ + d + e \\ + f \end{multline}`,
	`\begin{eqnarray} a &=& b \\ c &=& d \end{eqnarray}`,
	`\begin{flalign*} a &= b \end{flalign*}`,
	`\sum_{\substack{i < n \\ j < m}} a_{ij}`,
	`\sum_{\begin{subarray}{l} i \\ j \end{subarray}} x`,
	`\text{if \textbf{bold} and \textit{it} \& 50\% \$5 a\_b \{x\}}`,
	`\text{a~b \emph{c} \textrm{d} \textbackslash{} \ldots}`,
	`\boxed{E = mc^2} \quad \phantom{x} \hphantom{y} \vphantom{\frac{a}{b}}`,
	`a \not= b \quad \not\in`,
	`\overbrace{a + b + c}^{n} \quad \underbrace{x + y}_{m}`,
	`A \xrightarrow{f} B \xleftarrow[g]{h} C`,
	`{a \over b} + {n \choose k} + {x \atop y}`,
	`\cfrac{1}{1 + \cfrac{1}{x}}`,
	`a \pmod{n} \quad a \mod n \quad \pod{n}`,
	`\mathrel{R} \mathbin{\#} \mathop{T}`,
	`a \prec b \succeq c \hookrightarrow d \rightleftharpoons e \Longrightarrow f \nearrow g`,
	`\oiint_S \iiiint \sech x \operatorname*{arg\,max}_x`,
}

func TestExtendedCommandsLayOut(t *testing.T) {
	for _, src := range extendedSamples {
		for _, display := range []bool{false, true} {
			b := mustLayout(t, src, display)
			if b.Width <= 0 || b.Ascent+b.Descent <= 0 {
				t.Fatalf("%q 排出空盒子：%+v", src, b)
			}
		}
	}
}

func TestMiddleStretchesWithFence(t *testing.T) {
	plain := mustLayout(t, `\left( \frac{a}{b} | \frac{c}{d} \right)`, true)
	middle := mustLayout(t, `\left( \frac{a}{b} \middle| \frac{c}{d} \right)`, true)
	bar := mustLayout(t, `|`, true)
	if middle.Ascent+middle.Descent < plain.Ascent+plain.Descent-0.01 {
		t.Fatalf("\\middle 不应让外侧定界符变矮：%+v 对比 %+v", middle, plain)
	}
	// 中间定界符的高度应接近外侧括号，而不是普通竖线。
	tall := tallestMiddle(middle)
	if tall < (plain.Ascent+plain.Descent)*0.8 || tall <= (bar.Ascent+bar.Descent)*1.3 {
		t.Fatalf("\\middle| 没有随内容伸高：%v，外侧 %v", tall, plain.Ascent+plain.Descent)
	}
	for _, src := range []string{`a \middle| b`, `\left( {a \middle| b} \right)`, `\left( a \middle b \right)`} {
		if _, err := Layout(src, 16, true); err == nil {
			t.Fatalf("%q 应返回错误", src)
		}
	}
}

// tallestMiddle 返回第二层子盒子（括号内的列表）里最高的盒子高度。
func tallestMiddle(b *Box) float32 {
	var tall float32
	for _, k := range b.kids {
		for _, inner := range k.box.kids {
			if inner.box.Width < 8 {
				tall = max(tall, inner.box.Ascent+inner.box.Descent)
			}
		}
	}
	return tall
}

func TestTextCommands(t *testing.T) {
	root, err := parse(`\text{a \textbf{b} \& c\% {d} e~f}`)
	if err != nil {
		t.Fatal(err)
	}
	got := root.list[0].text
	if want := "a " + string(styled('b', varBold)) + " & c% d e f"; got != want {
		t.Fatalf("文字解析为 %q，应为 %q", got, want)
	}
	for _, src := range []string{`\text{a \frac{1}{2}}`, `\text{a`, `\text{\textbf{a}`} {
		if _, err := Layout(src, 16, false); err == nil {
			t.Fatalf("%q 应返回错误", src)
		}
	}
}

// commonCommands 逐项覆盖常用命令，每条都要在行内与块级样式下排出非空盒子并能导出 SVG。
var commonCommands = []string{
	// 重音与装饰
	`\hat{a} \bar{a} \vec{a} \tilde{a} \dot{a} \ddot{a} \dddot{a} \check{a} \breve{a} \acute{a} \grave{a} \mathring{a}`,
	`\overline{AB} \underline{AB} \widehat{abc} \widetilde{abc} \widecheck{abc}`,
	`\overrightarrow{AB} \overleftarrow{AB} \overleftrightarrow{AB}`,
	`\overbrace{a+b}^{n} \underbrace{a+b}_{m} \overbracket{a+b} \underbracket{a+b} \overparen{AB} \underparen{AB}`,
	// 字体
	`\mathbb{R} \mathcal{L} \mathfrak{g} \mathbf{v} \mathrm{d} \mathit{ab} \mathsf{T} \mathtt{x} \mathscr{F} \mathnormal{x}`,
	`\boldsymbol{\alpha} \bm{x} \mathbf{\Gamma} \boldsymbol{\nabla} \mathrm{\mu} \pmb{y} {\rm d}x {\bf v} {\it w} {\cal A}`,
	// 运算符、分式、二项式
	`\operatorname{rank} A \quad \operatorname*{arg\,max}_x f \quad \argmax_x \quad \mathop{\mathrm{lcm}}_{i} a_i`,
	`\frac{a}{b} \dfrac{a}{b} \tfrac{a}{b} \cfrac{a}{b} \binom{n}{k} \dbinom{n}{k} \tbinom{n}{k}`,
	// 叠放与带标注箭头
	`a \stackrel{\text{def}}{=} b \overset{?}{=} c \underset{x}{\to} d`,
	`A \xrightarrow{f} B \xleftarrow[g]{h} C \xrightarrow[\text{below}]{} D`,
	// 边框、颜色、删除线
	`\boxed{E = mc^2}`,
	`\color{red} a + b \quad {\color{blue} c} + \textcolor{green}{d} + \textcolor{#f80}{e} + \textcolor[HTML]{00AAFF}{f}`,
	`\cancel{x} + \bcancel{y} + \xcancel{z}`,
	// 间距
	`a\,b\:c\;d\!e\ f\quad g\qquad h~i \enspace j \thinspace k \medspace l \thickspace m \negthinspace n`,
	`a\hspace{1em}b\hspace*{-2pt}c\kern3mu d\mkern-1.5mu e\mskip 3mu f\hskip 0.5em g \hfill h`,
	`\phantom{x} \hphantom{y} \vphantom{\frac{a}{b}} \smash{\frac{a}{b}} \mathstrut \strut`,
	// 环境
	`\begin{array}{l|cr} a & b & c \\ \hline d & e & f \end{array}`,
	`\left[\begin{array}{cc|c} 1 & 0 & 2 \\ 0 & 1 & 3 \end{array}\right]`,
	`\begin{array}{|c|c|} \hline a & b \\ \hline c & d \\ \hline \end{array}`,
	`\begin{gather} a = b \\ c = d \end{gather} \begin{gather*} a \\ b \end{gather*} \begin{gathered} a \\ b \end{gathered}`,
	`\begin{align} a &= b \\ c &= d \end{align} \begin{align*} a &= b \end{align*} \begin{aligned} a &= b \end{aligned}`,
	`\begin{alignat}{2} a &= b & c &= d \end{alignat} \begin{alignat*}{2} a &= b & c &= d \end{alignat*}`,
	`\begin{equation} \begin{split} a &= b \\ &= c \end{split} \end{equation} \begin{equation*} x \end{equation*}`,
	`\begin{multline} a + b \\ + c \end{multline} \begin{multline*} a + b \\ + c \end{multline*}`,
	`\begin{smallmatrix} a & b \\ c & d \end{smallmatrix} \begin{psmallmatrix} a \\ b \end{psmallmatrix} \begin{bsmallmatrix} a \\ b \end{bsmallmatrix}`,
	`\begin{pmatrix*}[r] -1 & 2 \\ 3 & -4 \end{pmatrix*} \begin{bmatrix*} a \end{bmatrix*} \begin{matrix*}[l] a \\ bb \end{matrix*}`,
	`\begin{matrix} a \end{matrix} \begin{Bmatrix} a \end{Bmatrix} \begin{vmatrix} a \end{vmatrix} \begin{Vmatrix} a \end{Vmatrix} \begin{cases} a \end{cases}`,
	`\sum_{\substack{0 < i < n \\ i \ne j}} a_i`,
	// 编号、否定、空定界符
	`E = mc^2 \tag{1}`, `a = b \tag*{式 A}`, `x \label{eq:x} \nonumber \notag`,
	`\begin{align} a &= b \tag{1.1} \\ c &= d \tag{1.2} \end{align}`,
	`a \not= b \not< c \not\in D \not\subset E \not\equiv f \not\to g \not\sqsubseteq h`,
	`\left. \frac{df}{dx} \right|_{x=0} \quad \left\{ \frac{a}{b} \right. \quad \left\uparrow \frac{a}{b} \right\Downarrow \quad \left\lbrack x \right\rbrack`,
	`\big( \Big[ \bigg\{ \Bigg| \bigl( \bigr) \bigm| \Bigl\langle \Bigr\rangle \biggl\lfloor \biggr\rceil \big\uparrow`,
	// 文字
	`\text{当 $x > 0$ 时} \quad \text{if $a_i^2 = \frac{1}{2}$ then} \quad \textsf{sans} \texttt{mono} \textup{up}`,
	// AMS 符号
	`a \leqslant b \geqslant c \lesssim d \gtrsim e \triangleq f \coloneqq g \vDash h \nrightarrow i \leadsto j \twoheadrightarrow k`,
	`a \ltimes b \rtimes c \boxplus d \circledast e \dotplus f \quad \Box \Diamond \blacksquare \bigstar \complement \varGamma \llbracket x \rrbracket`,
	`\tg x \ctg x \arctg x \sh x \ch x \th x \plim_{n} x_n \argmin_x`,
}

func TestCommonCommands(t *testing.T) {
	for _, src := range commonCommands {
		for _, display := range []bool{false, true} {
			b := mustLayout(t, src, display)
			if b.Width <= 0 || b.Ascent+b.Descent <= 0 {
				t.Fatalf("%q 排出空盒子：%+v", src, b)
			}
			if _, err := b.SVG(); err != nil {
				t.Fatalf("%q 导出失败：%v", src, err)
			}
		}
	}
}

// 表里的每个字符都必须在内嵌字体里有字形，否则画出来是缺字方框。
func TestSymbolTablesHaveGlyphs(t *testing.T) {
	f, err := loadFont()
	if err != nil {
		t.Fatal(err)
	}
	check := func(what string, r rune) {
		t.Helper()
		if _, ok := f.glyph(r); !ok {
			t.Errorf("%s 对应的 %U 不在数学字体里", what, r)
		}
	}
	for name, s := range symbols {
		check(`\`+name, s.r)
	}
	for name, s := range bigOps {
		check(`\`+name, s.r)
	}
	for name, a := range accents {
		if a.r != 0 {
			check(`\`+name, a.r)
		}
	}
	for name, r := range delimiters {
		if r != 0 {
			check("定界符 "+name, r)
		}
	}
	for from, to := range negated {
		check(`\not`+string(from), to)
	}
	for v := varMath; v <= varMono; v++ {
		for _, r := range "ABCEFHILMNPQRZaeghoz019" {
			check("字母变体", styled(r, v))
		}
		for _, r := range []rune{0x0393, 0x03A9, 0x2207, 0x1D6FC, 0x1D714, 0x1D71B} {
			check("希腊字母变体", styledSymbol(r, v))
		}
	}
}

func TestUnknownInputFallsBack(t *testing.T) {
	for _, src := range []string{
		`\usepackage{amsmath}`, `\newcommand{\R}{\mathbb{R}}`, `\def\x{1}`, `\begin{tikzpicture} \end{tikzpicture}`,
		`\color{nosuchcolor} x`, `\textcolor{#12}{x}`, `\color[rgb]{1,0,0} x`, `\textcolor{red}`,
		`\hspace{abc}`, `\kern`, `\hspace{1em`, `\kern 1e99em`,
		`{a \tag{1}}`, `\frac{a \tag{1}}{b}`, `\begin{pmatrix} a \tag{1} \end{pmatrix}`, `\tag{$x$}`,
		`a \hline b`, `\sum_{\substack{\hline a}}`, `\begin{pmatrix*}[x] a \end{pmatrix*}`,
		`\text{a $x}`, `\text{$\frac{1}$}`, `\text{$a } b$}`, `\operatorname{a $x$}`,
		`\cancel`, `\not`, `\label`, `\large x`, `\mathbb`, `\begin{array} a \end{array}`,
	} {
		for _, display := range []bool{false, true} {
			if b, err := Layout(src, 16, display); err == nil {
				t.Fatalf("%q 应返回错误，得到 %+v", src, b)
			}
		}
	}
}

func TestExtendedNeverPanics(t *testing.T) {
	pieces := []string{`\color`, `{red}`, `\textcolor`, `\tag`, `{1}`, `\hline`, `\begin{array}`, `{c|c}`, `\end{array}`, `\text{`, `$`, `}`, `{`,
		`\cancel`, `\not`, `=`, `\hspace`, `{1em}`, `\kern`, `1mu`, `\rm`, `\substack`, `&`, `\\`, `\xrightarrow`, `[`, `]`, `\overset`, `\boldsymbol`,
		`\alpha`, `x`, `\begin{align}`, `\end{align}`, `\left.`, `\right|`, `\middle|`, `\smash`, `\begin{pmatrix*}`, `[r]`, `\end{pmatrix*}`, "中", `\`}
	rng := rand.New(rand.NewSource(11))
	for i := 0; i < 6000; i++ {
		var src strings.Builder
		for n := rng.Intn(14); n >= 0; n-- {
			src.WriteString(pieces[rng.Intn(len(pieces))])
		}
		for _, display := range []bool{false, true} {
			b, err := Layout(src.String(), 16, display)
			if err != nil {
				continue
			}
			if b == nil {
				t.Fatalf("%q 没有错误却返回空盒子", src.String())
			}
			// 排得出来的公式导出时也不能 panic；失败只允许以错误的形式返回。
			if svg, err := b.SVG(); err == nil && !strings.HasSuffix(svg, "</svg>") {
				t.Fatalf("%q 的 SVG 不完整", src.String())
			}
		}
	}
}

func TestFontVariants(t *testing.T) {
	text := func(src string) string {
		t.Helper()
		root, err := parse(src)
		if err != nil {
			t.Fatalf("%q：%v", src, err)
		}
		var out strings.Builder
		var walk func(n *node)
		walk = func(n *node) {
			if n == nil {
				return
			}
			out.WriteString(n.text)
			for _, k := range n.list {
				walk(k)
			}
			if n.table != nil {
				for _, row := range n.table.rows {
					for _, cell := range row {
						for _, k := range cell {
							walk(k)
						}
					}
				}
			}
		}
		walk(root)
		return out.String()
	}
	for src, want := range map[string]string{
		`\mathsf{A1}`:                           "\U0001D5A0\U0001D7E3",
		`\mathtt{a}`:                            "\U0001D68A",
		`\mathbb{R}`:                            "ℝ",
		`\mathcal{L}`:                           "ℒ",
		`\mathfrak{g}`:                          "\U0001D524",
		`\boldsymbol{\alpha}`:                   "\U0001D736",
		`\mathbf{\Gamma}`:                       "\U0001D6AA",
		`\boldsymbol{\nabla}`:                   "\U0001D735",
		`\mathrm{\mu}`:                          "μ",
		`\mathbf{+}`:                            "+",
		`{\rm d}x`:                              "d\U0001D465",
		`{\bf a}b`:                              "\U0001D41A\U0001D44F",
		`\mathrm{a{\it b}c}`:                    "a\U0001D44Fc",
		`\begin{matrix} \bf a & b \end{matrix}`: "\U0001D41A\U0001D44F",
	} {
		if got := text(src); got != want {
			t.Errorf("%q 解析为 %q，应为 %q", src, got, want)
		}
	}
}

func TestNotUsesNegatedGlyph(t *testing.T) {
	root, err := parse(`a \not= b \not\in C`)
	if err != nil {
		t.Fatal(err)
	}
	if got := root.list[1]; got.text != "≠" || got.cls != clsRel {
		t.Fatalf(`\not= 应换成 ≠ 并保持关系符类别：%+v`, got)
	}
	if got := root.list[3]; got.text != "∉" || got.cls != clsRel {
		t.Fatalf(`\not\in 应换成 ∉：%+v`, got)
	}
	// 没有现成字形时叠一条斜线，宽度不变。
	plain, struck := mustLayout(t, `\sqsubseteq`, true), mustLayout(t, `\not\sqsubseteq`, true)
	if struck.Width != plain.Width || len(struck.kids) != 2 {
		t.Fatalf("叠斜线不应改变宽度：%+v 对比 %+v", struck, plain)
	}
}

func TestOversetKeepsRelationSpacing(t *testing.T) {
	plain := mustLayout(t, `a = b`, true)
	for _, src := range []string{`a \overset{?}{=} b`, `a \underset{x}{=} b`, `a \stackrel{?}{=} b`} {
		if got := mustLayout(t, src, true); got.Width < plain.Width-0.01 {
			t.Fatalf("%q 应保留关系符两侧的间距：%v 对比 %v", src, got.Width, plain.Width)
		}
	}
	ord := mustLayout(t, `a x b`, true)
	if got := mustLayout(t, `a \overset{?}{x} b`, true); got.Width > ord.Width+0.01 {
		t.Fatalf("普通符号上叠标注不应多出间距：%v 对比 %v", got.Width, ord.Width)
	}
}

func TestSpacingCommands(t *testing.T) {
	base := mustLayout(t, `ab`, false).Width
	for src, em := range map[string]float32{
		`a\quad b`: 1, `a\qquad b`: 2, `a\,b`: 3.0 / 18, `a\!b`: -3.0 / 18, `a\hspace{1em}b`: 1, `a\hspace*{2em}b`: 2,
		`a\kern-0.5em b`: -0.5, `a\mkern18mu b`: 1, `a\mskip 9mu b`: 0.5, `a\hskip10pt b`: 1, `a\hfill b`: 0, `a~b`: 6.0 / 18,
	} {
		if got := mustLayout(t, src, false).Width - base; got < em*16-0.01 || got > em*16+0.01 {
			t.Errorf("%q 的间距是 %v，应为 %v", src, got, em*16)
		}
	}
}

func TestColor(t *testing.T) {
	tints := func(b *Box) (list [][3]uint8) {
		var walk func(b *Box)
		walk = func(b *Box) {
			if b.tinted {
				list = append(list, b.tint)
			}
			for _, k := range b.kids {
				walk(k.box)
			}
		}
		walk(b)
		return list
	}
	b := mustLayout(t, `a + \textcolor{red}{b} + c`, true)
	if got := tints(b); len(got) != 1 || got[0] != [3]uint8{255, 0, 0} {
		t.Fatalf(`\textcolor 应只染一个盒子：%v`, got)
	}
	if plain := mustLayout(t, `a + b + c`, true); b.Width != plain.Width {
		t.Fatalf("染色不应改变排版：%v 对比 %v", b.Width, plain.Width)
	}
	// \color 作用到所在组的结尾，间距与不染色时一致。
	b = mustLayout(t, `a {\color{#0af} + b = c} + d`, true)
	if got := tints(b); len(got) != 4 || got[0] != [3]uint8{0, 170, 255} {
		t.Fatalf(`\color 应染组内其余四个原子：%v`, got)
	}
	if plain := mustLayout(t, `a {+ b = c} + d`, true); b.Width != plain.Width {
		t.Fatalf("染色不应改变排版：%v 对比 %v", b.Width, plain.Width)
	}
	// 内层颜色优先，SVG 用分组填色，其余仍是 currentColor。
	svg, err := mustLayout(t, `\textcolor{blue}{a \textcolor[HTML]{FF8800}{b}} c`, true).SVG()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(svg, `<g fill="#0000ff">`) != 1 || strings.Count(svg, `<g fill="#ff8800">`) != 1 || strings.Count(svg, "</g>") != 2 {
		t.Fatalf("SVG 的颜色分组不对：%s", svg)
	}
	if !strings.Contains(svg, `fill="currentColor"`) {
		t.Fatal("SVG 的默认颜色应为 currentColor")
	}
}

func TestTag(t *testing.T) {
	plain, tagged := mustLayout(t, `E = mc^2`, true), mustLayout(t, `E = mc^2 \tag{1}`, true)
	if tagged.Width < plain.Width+2*16 {
		t.Fatalf("编号应排在公式右侧并留出间距：%v 对比 %v", tagged.Width, plain.Width)
	}
	root, err := parse(`\begin{align} a &= b \tag{1} \\ c &= d \tag*{A} \\ e &= f \end{align}`)
	if err != nil {
		t.Fatal(err)
	}
	rows := root.list[0].table.rows
	if got := rowTag(rows[0]); got == nil || got.text != "(1)" {
		t.Fatalf(`\tag 应带括号：%+v`, got)
	}
	if got := rowTag(rows[1]); got == nil || got.text != "A" {
		t.Fatalf(`\tag* 不带括号：%+v`, got)
	}
	if rowTag(rows[2]) != nil {
		t.Fatal("没有 \\tag 的行不应有编号")
	}
}

func TestArrayLines(t *testing.T) {
	root, err := parse(`\begin{array}{|l|cr|} \hline a & b & c \\ \hline\hline d & e & f \\ \hline \end{array}`)
	if err != nil {
		t.Fatal(err)
	}
	table := root.list[0].table
	if len(table.rows) != 2 || table.spec.aligns != "lcr" {
		t.Fatalf("行列解析不对：%d 行，对齐 %q", len(table.rows), table.spec.aligns)
	}
	if got := table.spec.vlines; len(got) != 3 || got[0] != 0 || got[1] != 1 || got[2] != 3 {
		t.Fatalf("竖线位置不对：%v", got)
	}
	if got := table.hlines; len(got) != 4 || got[0] != 0 || got[1] != 1 || got[2] != 1 || got[3] != 2 {
		t.Fatalf("横线位置不对：%v", got)
	}
	lined := mustLayout(t, `\begin{array}{|c|c|} \hline a & b \\ \hline c & d \\ \hline \end{array}`, true)
	plain := mustLayout(t, `\begin{array}{cc} a & b \\ c & d \end{array}`, true)
	if len(lined.rules) != 6 || len(plain.rules) != 0 {
		t.Fatalf("应有 3 条横线和 3 条竖线：%d，无线表格 %d", len(lined.rules), len(plain.rules))
	}
	if lined.Width <= plain.Width || lined.Ascent+lined.Descent <= plain.Ascent+plain.Descent {
		t.Fatalf("带线的表格应在线旁留出空隙：%+v 对比 %+v", lined, plain)
	}
	for _, r := range lined.rules {
		if r.x < -0.01 || r.x+r.w > lined.Width+0.01 || r.y < -lined.Ascent-0.01 || r.y+r.h > lined.Descent+0.01 {
			t.Fatalf("线超出了表格范围：%+v，表格 %+v", r, lined)
		}
	}
}

func TestTextWithInlineMath(t *testing.T) {
	root, err := parse(`\text{当 $x^2 > 0$ 时成立}`)
	if err != nil {
		t.Fatal(err)
	}
	list := root.list[0].list
	if len(list) != 3 || list[0].text != "当 " || list[1].kind != nGroup || list[2].text != " 时成立" {
		t.Fatalf("文字与公式应分成三段：%+v", list)
	}
	// 文字里的公式沿用所在位置的样式：放进下标时跟着缩小。
	if big, small := mustLayout(t, `\text{a $x$}`, false), mustLayout(t, `y_{\text{a $x$}}`, false); small.Width >= big.Width+mustLayout(t, `y`, false).Width {
		t.Fatalf("下标里的文字应缩小：%v 对比 %v", small.Width, big.Width)
	}
}

func TestCancelAndSmash(t *testing.T) {
	plain := mustLayout(t, `\frac{a}{b}`, true)
	for src, strokes := range map[string]int{`\cancel{\frac{a}{b}}`: 1, `\bcancel{\frac{a}{b}}`: 1, `\xcancel{\frac{a}{b}}`: 2} {
		b := mustLayout(t, src, true)
		if len(b.paths) != strokes || b.Width != plain.Width || b.Ascent <= plain.Ascent {
			t.Fatalf("%q 应画 %d 条删除线且不改变宽度：%+v", src, strokes, b)
		}
	}
	if b := mustLayout(t, `\smash{\frac{a}{b}}`, true); b.Width != plain.Width || b.Ascent != 0 || b.Descent != 0 {
		t.Fatalf(`\smash 应保留宽度、不占高度：%+v`, b)
	}
}

func TestSVGMetrics(t *testing.T) {
	for _, src := range []string{`x`, `\frac{a}{b}`, `\sqrt{x^2 + 1}`, `\begin{pmatrix} a & b \\ c & d \end{pmatrix}`} {
		b := mustLayout(t, src, true)
		svg, err := b.SVG()
		if err != nil {
			t.Fatal(err)
		}
		width, height := svgNumber(b.Width), svgNumber(b.Ascent+b.Descent)
		want := `<svg xmlns="http://www.w3.org/2000/svg" width="` + width + `" height="` + height + `" viewBox="0 0 ` + width + ` ` + height + `" fill="currentColor">`
		if !strings.HasPrefix(svg, want) {
			t.Fatalf("%q 的 SVG 尺寸应等于盒子尺寸：%s", src, svg[:min(len(svg), 200)])
		}
		if err := xml.Unmarshal([]byte(svg), new(struct{})); err != nil {
			t.Fatalf("%q 的 SVG 不是合法 XML：%v", src, err)
		}
	}
	// 基线在顶部下方 Ascent 处：分数线（唯一的矩形）应位于基线上方的数学轴附近。
	frac := mustLayout(t, `\frac{a}{b}`, true)
	svg, _ := frac.SVG()
	var doc struct {
		Rects []struct {
			Y float32 `xml:"y,attr"`
		} `xml:"rect"`
	}
	if err := xml.Unmarshal([]byte(svg), &doc); err != nil || len(doc.Rects) != 1 {
		t.Fatalf("分式应恰有一个矩形：%v %+v", err, doc)
	}
	if y := doc.Rects[0].Y; y >= frac.Ascent || y < frac.Ascent-16*0.5 {
		t.Fatalf("分数线的位置 %v 应略高于基线 %v", y, frac.Ascent)
	}
}

func TestSVGFallbackText(t *testing.T) {
	b := mustLayout(t, `\text{速度 v} = \frac{\text{路程<&>"}}{\text{时间}}`, true)
	svg, err := b.SVG()
	if err != nil {
		t.Fatalf("含中文的公式应能导出：%v", err)
	}
	var doc struct {
		Paths []struct{} `xml:"path"`
		Texts []struct {
			X      float32 `xml:"x,attr"`
			Y      float32 `xml:"y,attr"`
			Size   float32 `xml:"font-size,attr"`
			Family string  `xml:"font-family,attr"`
			Length float32 `xml:"textLength,attr"`
			Body   string  `xml:",chardata"`
		} `xml:"text"`
	}
	if err := xml.Unmarshal([]byte(svg), &doc); err != nil {
		t.Fatalf("SVG 不是合法 XML：%v\n%s", err, svg)
	}
	// 中文各成一段 <text>；拉丁字母、符号（含 XML 的特殊字符）和分数线仍是轮廓与矩形。
	if len(doc.Texts) != 3 || doc.Texts[0].Body != "速度" || doc.Texts[1].Body != "路程" || doc.Texts[2].Body != "时间" {
		t.Fatalf("缺字部分应输出三段文字：%+v", doc.Texts)
	}
	if len(doc.Paths) != 6 || !strings.Contains(svg, "<rect") {
		t.Fatalf("其余部分应保持轮廓：%d 条路径", len(doc.Paths))
	}
	for _, text := range doc.Texts {
		if !strings.Contains(text.Family, "PingFang SC") || !strings.Contains(text.Family, "Microsoft YaHei") || !strings.HasSuffix(text.Family, "sans-serif") {
			t.Fatalf("字体栈不对：%q", text.Family)
		}
		// 两个汉字约占两个字宽，且整段落在画布内。
		if text.Length < text.Size*1.6 || text.Length > text.Size*2.4 || text.X < 0 || text.X+text.Length > b.Width+0.01 || text.Y <= 0 || text.Y > b.Ascent+b.Descent {
			t.Fatalf("文字的位置或宽度不合理：%+v，盒子 %+v", text, b)
		}
	}
	if doc.Texts[0].Size != 16 || doc.Texts[0].Y != b.Ascent {
		t.Fatalf("主行文字应在基线上、使用正文字号：%+v", doc.Texts[0])
	}

	// 文字引擎不可用时按字体度量估算位置，结果仍然可用。
	mixed := mustLayout(t, `\text{a中文b}`, true)
	mixed.glyphs = nil
	mixed.Width = 0
	svg, err = mixed.SVG()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(svg, `textLength="32.0000"`) || strings.Count(svg, "<path") != 2 {
		t.Fatalf("估算宽度应为两个字宽，两侧字母各一条轮廓：%s", svg)
	}

	// 确实无法显示的字符才返回错误。
	bad := mustLayout(t, `x`, true)
	bad.svgText = "a\u0007"
	if _, err := bad.SVG(); err == nil {
		t.Fatal("控制字符应返回错误")
	}
}

func TestSVG(t *testing.T) {
	for _, s := range renderSamples {
		b := mustLayout(t, s.src, s.display)
		svg, err := b.SVG()
		if err != nil {
			t.Fatalf("%q 导出失败：%v", s.src, err)
		}
		if !strings.HasPrefix(svg, `<svg xmlns="http://www.w3.org/2000/svg"`) || !strings.HasSuffix(svg, "</svg>") || !strings.Contains(svg, "<path d=") {
			t.Fatalf("%q 的 SVG 结构不完整", s.src)
		}
		if err := xml.Unmarshal([]byte(svg), new(struct{})); err != nil {
			t.Fatalf("%q 的 SVG 不是合法 XML：%v", s.src, err)
		}
		banned := []string{"NaN", "Inf", "href", "<script", "<image", "<style"}
		if !hasFallbackText(s.src) {
			// 字体里都有的公式完全由轮廓组成，不依赖任何系统字体。
			banned = append(banned, "<text", "font-family")
		}
		for _, word := range banned {
			if strings.Contains(svg, word) {
				t.Fatalf("%q 的 SVG 含有 %s", s.src, word)
			}
		}
	}
	for _, src := range extendedSamples {
		if _, err := mustLayout(t, src, true).SVG(); err != nil {
			t.Fatalf("%q 导出失败：%v", src, err)
		}
	}
	var nilBox *Box
	if _, err := nilBox.SVG(); err == nil {
		t.Fatal("空盒子应返回错误")
	}
}

// hasFallbackText 判断源码里有没有数学字体不含的字符。
func hasFallbackText(src string) bool {
	f, _ := loadFont()
	for _, r := range src {
		if _, ok := f.glyph(r); !ok && !unicode.IsSpace(r) {
			return true
		}
	}
	return false
}
