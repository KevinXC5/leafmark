package mathlayout

import (
	"image/png"
	"os"
	"testing"

	"github.com/egoist/mygo/ui"
)

// renderSamples 是离屏渲染用的样例：覆盖各类结构，便于目视核对。
var renderSamples = []struct {
	src     string
	display bool
}{
	{`Q_n = Q \cdot r^n, \qquad 0 < r < 1`, true},
	{`s = vt`, false},
	{`x = \frac{-b \pm \sqrt{b^2 - 4ac}}{2a}`, true},
	{`\sum_{i=1}^{n} i^2 = \frac{n(n+1)(2n+1)}{6}, \quad \prod_{k=1}^n k = n!`, true},
	{`\int_0^\infty e^{-x^2}\,dx = \frac{\sqrt{\pi}}{2}, \quad \oint_C \vec{F}\cdot d\vec{r}`, true},
	{`\lim_{x\to 0} \frac{\sin x}{x} = 1, \quad \max_{i} a_i, \quad f'(x), \; f''_i`, true},
	{`\sum_{i=1}^n a_i \;\; \int_a^b f \;\; \lim_{n\to\infty} x_n \;\; \frac{a}{b} \;\; \sqrt[3]{x}`, false},
	{`\left( \frac{a}{b} \right)^2 + \left[ \sum_{i} x_i \right] + \left\{ \frac{\frac{1}{2}}{\frac{3}{4}} \right\} + \left| x \right| + \left\langle \frac{u}{v} \right\rangle`, true},
	{`\begin{pmatrix} a & b \\ c & d \end{pmatrix} \begin{bmatrix} 1 & 0 & 0 \\ 0 & 1 & 0 \\ 0 & 0 & 1 \end{bmatrix} \begin{vmatrix} x & y \\ z & w \end{vmatrix}`, true},
	{`f(x) = \begin{cases} x^2 & \text{if } x \ge 0 \\ -x & \text{otherwise} \end{cases}`, true},
	{`\begin{aligned} a &= b + c \\ d + e &= f \end{aligned}`, true},
	{`\hat{x} \; \bar{y} \; \vec{v} \; \dot{a} \; \ddot{a} \; \tilde{n} \; \overline{AB} \; \widehat{xyz} \; \overrightarrow{ABCDE} \; \hat{f}`, true},
	{`\alpha\beta\gamma\delta\epsilon\varepsilon\zeta\eta\theta\vartheta\iota\kappa\lambda\mu\nu\xi\pi\rho\sigma\tau\upsilon\phi\varphi\chi\psi\omega`, true},
	{`\Gamma\Delta\Theta\Lambda\Xi\Pi\Sigma\Upsilon\Phi\Psi\Omega \quad \mathbb{R} \subset \mathbb{C}, \mathbb{NZQ} \quad \mathbf{v} \; \mathrm{d}x \; \mathit{diff} \; \mathcal{L}`, true},
	{`a \leq b \geq c \neq d \approx e \equiv f \sim g \propto h \in A \notin B \subset C \subseteq D \cup E \cap F`, true},
	{`x \to y \rightarrow z \leftarrow w \Rightarrow u \Leftarrow v \leftrightarrow s \Leftrightarrow t \mapsto r, \forall x \exists y, \partial \nabla \infty`, true},
	{`a \times b \div c \pm d \mp e \circ f, 90\degree, \angle A \perp B \parallel C, 1, 2, \dots, n; a + \cdots + z`, true},
	{`e^{i\pi} + 1 = 0, \quad x_i^2, \quad x^{a+b}, \quad x_{i_j}^{2^k}, \quad \binom{n}{k}, \quad \tfrac12 \dfrac12`, false},
	{`\bigcup_{i=1}^n A_i \bigcap_{j} B_j \quad \sqrt{\frac{a}{b}} \quad \sqrt{\sum_{i=1}^{n} \left(\frac{x_i}{y_i}\right)^2}`, true},
	{`a \\ b + c`, true},
	{`\text{速度} = \frac{\text{路程}}{\text{时间}}`, true},
}

// TestRenderSamples 在离屏窗口里绘制全部样例，确认 Paint 不 panic 且确实画出了内容。
// 设置环境变量 MATHLAYOUT_PNG 时把画面写到该路径，供目视核对。
func TestRenderSamples(t *testing.T) {
	const width, rowHeight = 1100, 96
	height := rowHeight * len(renderSamples)
	boxes := make([]*Box, len(renderSamples))
	for i, s := range renderSamples {
		size := float32(20)
		b, err := Layout(s.src, size, s.display)
		if err != nil {
			t.Fatalf("样例 %q 排版失败：%v", s.src, err)
		}
		boxes[i] = b
	}
	view := func(c *ui.Context) {
		ui.Box(c).Size(width, float32(height)).Background(ui.RGB(255, 255, 255)).Draw(func(p *ui.Painter, r ui.Rect) {
			for i, b := range boxes {
				mid := r.Y + float32(i*rowHeight) + rowHeight/2
				baseline := mid + (b.Ascent-b.Descent)/2
				b.Paint(p, r.X+24, baseline, ui.RGB(20, 20, 20))
			}
		})
	}
	tt := ui.NewTester(view, width, height)
	tt.SetScale(2)
	tt.Frame()
	img := tt.Image()
	dark := 0
	for i := 0; i < len(img.Pix); i += 4 {
		if img.Pix[i] < 128 {
			dark++
		}
	}
	if dark == 0 {
		t.Fatalf("离屏画面里没有任何深色像素，公式没有画出来")
	}
	if path := os.Getenv("MATHLAYOUT_PNG"); path != "" {
		f, err := os.Create(path)
		if err != nil {
			t.Fatalf("创建 %s：%v", path, err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			t.Fatalf("写入 %s：%v", path, err)
		}
	}
}
