package mathlayout

// class 是 TeX 的原子类别，决定相邻原子之间的间距。
type class uint8

const (
	clsOrd class = iota
	clsOp
	clsBin
	clsRel
	clsOpen
	clsClose
	clsPunct
	clsInner
	clsNone // 间距、样式切换等不参与间距计算的节点
)

type symbol struct {
	r   rune
	cls class
}

// symbols 是“命令名 → 字符”的对照表，字符全部取自内嵌数学字体。
var symbols = map[string]symbol{
	// 小写希腊字母：数学斜体区
	"alpha": {0x1D6FC, clsOrd}, "beta": {0x1D6FD, clsOrd}, "gamma": {0x1D6FE, clsOrd},
	"delta": {0x1D6FF, clsOrd}, "varepsilon": {0x1D700, clsOrd}, "zeta": {0x1D701, clsOrd},
	"eta": {0x1D702, clsOrd}, "theta": {0x1D703, clsOrd}, "iota": {0x1D704, clsOrd},
	"kappa": {0x1D705, clsOrd}, "lambda": {0x1D706, clsOrd}, "mu": {0x1D707, clsOrd},
	"nu": {0x1D708, clsOrd}, "xi": {0x1D709, clsOrd}, "omicron": {0x1D70A, clsOrd},
	"pi": {0x1D70B, clsOrd}, "rho": {0x1D70C, clsOrd}, "varsigma": {0x1D70D, clsOrd},
	"sigma": {0x1D70E, clsOrd}, "tau": {0x1D70F, clsOrd}, "upsilon": {0x1D710, clsOrd},
	"varphi": {0x1D711, clsOrd}, "chi": {0x1D712, clsOrd}, "psi": {0x1D713, clsOrd},
	"omega": {0x1D714, clsOrd}, "partial": {0x1D715, clsOrd}, "epsilon": {0x1D716, clsOrd},
	"vartheta": {0x1D717, clsOrd}, "varkappa": {0x1D718, clsOrd}, "phi": {0x1D719, clsOrd},
	"varrho": {0x1D71A, clsOrd}, "varpi": {0x1D71B, clsOrd},
	// 大写希腊字母：正体
	"Gamma": {0x0393, clsOrd}, "Delta": {0x0394, clsOrd}, "Theta": {0x0398, clsOrd},
	"Lambda": {0x039B, clsOrd}, "Xi": {0x039E, clsOrd}, "Pi": {0x03A0, clsOrd},
	"Sigma": {0x03A3, clsOrd}, "Upsilon": {0x03A5, clsOrd}, "Phi": {0x03A6, clsOrd},
	"Psi": {0x03A8, clsOrd}, "Omega": {0x03A9, clsOrd},
	// 普通符号
	"infty": {0x221E, clsOrd}, "nabla": {0x2207, clsOrd}, "forall": {0x2200, clsOrd},
	"exists": {0x2203, clsOrd}, "nexists": {0x2204, clsOrd}, "emptyset": {0x2205, clsOrd},
	"varnothing": {0x2205, clsOrd}, "hbar": {0x210F, clsOrd}, "ell": {0x2113, clsOrd},
	"Re": {0x211C, clsOrd}, "Im": {0x2111, clsOrd}, "aleph": {0x2135, clsOrd},
	"imath": {0x1D6A4, clsOrd}, "jmath": {0x1D6A5, clsOrd},
	"degree": {0x00B0, clsOrd}, "prime": {0x2032, clsOrd}, "angle": {0x2220, clsOrd},
	"triangle": {0x25B3, clsOrd}, "square": {0x25A1, clsOrd}, "neg": {0x00AC, clsOrd},
	"lnot": {0x00AC, clsOrd}, "top": {0x22A4, clsOrd}, "bot": {0x22A5, clsOrd},
	"vdots": {0x22EE, clsOrd}, "backslash": {'\\', clsOrd}, "vert": {'|', clsOrd},
	"Vert": {0x2016, clsOrd}, "|": {0x2016, clsOrd},
	"%": {'%', clsOrd}, "#": {'#', clsOrd}, "&": {'&', clsOrd}, "$": {'$', clsOrd}, "_": {'_', clsOrd},
	"dots": {0x2026, clsInner}, "ldots": {0x2026, clsInner}, "cdots": {0x22EF, clsInner},
	"ddots": {0x22F1, clsInner},
	// 二元运算符
	"cdot": {0x22C5, clsBin}, "times": {0x00D7, clsBin}, "div": {0x00F7, clsBin},
	"pm": {0x00B1, clsBin}, "mp": {0x2213, clsBin}, "cup": {0x222A, clsBin},
	"cap": {0x2229, clsBin}, "setminus": {0x2216, clsBin}, "circ": {0x2218, clsBin},
	"bullet": {0x2219, clsBin}, "star": {0x22C6, clsBin}, "ast": {0x2217, clsBin},
	"oplus": {0x2295, clsBin}, "ominus": {0x2296, clsBin}, "otimes": {0x2297, clsBin},
	"odot": {0x2299, clsBin}, "wedge": {0x2227, clsBin}, "land": {0x2227, clsBin},
	"vee": {0x2228, clsBin}, "lor": {0x2228, clsBin},
	// 关系符
	"leq": {0x2264, clsRel}, "le": {0x2264, clsRel}, "geq": {0x2265, clsRel},
	"ge": {0x2265, clsRel}, "neq": {0x2260, clsRel}, "ne": {0x2260, clsRel},
	"ll": {0x226A, clsRel}, "gg": {0x226B, clsRel}, "approx": {0x2248, clsRel},
	"equiv": {0x2261, clsRel}, "sim": {0x223C, clsRel}, "simeq": {0x2243, clsRel},
	"cong": {0x2245, clsRel}, "propto": {0x221D, clsRel}, "in": {0x2208, clsRel},
	"notin": {0x2209, clsRel}, "ni": {0x220B, clsRel}, "subset": {0x2282, clsRel},
	"supset": {0x2283, clsRel}, "subseteq": {0x2286, clsRel}, "supseteq": {0x2287, clsRel},
	"perp": {0x22A5, clsRel}, "parallel": {0x2225, clsRel}, "mid": {0x2223, clsRel},
	"vdash": {0x22A2, clsRel}, "models": {0x22A8, clsRel},
	"to": {0x2192, clsRel}, "rightarrow": {0x2192, clsRel}, "leftarrow": {0x2190, clsRel},
	"gets": {0x2190, clsRel}, "Rightarrow": {0x21D2, clsRel}, "Leftarrow": {0x21D0, clsRel},
	"leftrightarrow": {0x2194, clsRel}, "Leftrightarrow": {0x21D4, clsRel},
	"mapsto": {0x21A6, clsRel}, "uparrow": {0x2191, clsRel}, "downarrow": {0x2193, clsRel},
	"longrightarrow": {0x27F6, clsRel}, "longleftarrow": {0x27F5, clsRel},
	"implies": {0x27F9, clsRel}, "iff": {0x27FA, clsRel},
	"therefore": {0x2234, clsRel}, "because": {0x2235, clsRel},
	// 定界符与标点
	"{": {'{', clsOpen}, "lbrace": {'{', clsOpen}, "}": {'}', clsClose}, "rbrace": {'}', clsClose},
	"langle": {0x27E8, clsOpen}, "rangle": {0x27E9, clsClose},
	"lfloor": {0x230A, clsOpen}, "rfloor": {0x230B, clsClose},
	"lceil": {0x2308, clsOpen}, "rceil": {0x2309, clsClose},
	"lvert": {'|', clsOpen}, "rvert": {'|', clsClose},
	"lVert": {0x2016, clsOpen}, "rVert": {0x2016, clsClose},
	"colon": {':', clsPunct},
}

// chars 是直接键入的 ASCII 符号；字母与数字另按字体变体映射。
var chars = map[rune]symbol{
	'+': {'+', clsBin}, '-': {0x2212, clsBin}, '*': {0x2217, clsBin},
	'=': {'=', clsRel}, '<': {'<', clsRel}, '>': {'>', clsRel}, ':': {':', clsRel},
	',': {',', clsPunct}, ';': {';', clsPunct},
	'.': {'.', clsOrd}, '!': {'!', clsOrd}, '?': {'?', clsOrd}, '|': {'|', clsOrd},
	'/': {'/', clsOrd}, '"': {'"', clsOrd}, '@': {'@', clsOrd}, '`': {'`', clsOrd},
	'(': {'(', clsOpen}, '[': {'[', clsOpen}, ')': {')', clsClose}, ']': {']', clsClose},
}

// runeClass 让直接键入的 Unicode 数学符号（如 ≤、×）得到与对应命令相同的间距。
var runeClass = func() map[rune]class {
	m := map[rune]class{}
	for _, s := range symbols {
		if s.r > 0x7F {
			m[s.r] = s.cls
		}
	}
	return m
}()

type bigOp struct {
	r      rune
	limits bool // 块级样式下上下限放在正上正下
}

var bigOps = map[string]bigOp{
	"sum": {0x2211, true}, "prod": {0x220F, true}, "coprod": {0x2210, true},
	"bigcup": {0x22C3, true}, "bigcap": {0x22C2, true}, "bigvee": {0x22C1, true},
	"bigwedge": {0x22C0, true}, "bigoplus": {0x2A01, true}, "bigotimes": {0x2A02, true},
	"bigodot": {0x2A00, true}, "bigsqcup": {0x2A06, true}, "biguplus": {0x2A04, true},
	"int": {0x222B, false}, "iint": {0x222C, false}, "iiint": {0x222D, false},
	"oint": {0x222E, false},
}

// functions 是正体函数名；值为 true 的在块级样式下把下标放到正下方。
var functions = map[string]bool{
	"sin": false, "cos": false, "tan": false, "cot": false, "sec": false, "csc": false,
	"arcsin": false, "arccos": false, "arctan": false,
	"sinh": false, "cosh": false, "tanh": false, "coth": false,
	"log": false, "ln": false, "lg": false, "exp": false,
	"arg": false, "deg": false, "dim": false, "hom": false, "ker": false,
	"lim": true, "limsup": true, "liminf": true, "max": true, "min": true,
	"sup": true, "inf": true, "det": true, "gcd": true, "Pr": true,
}

// functionText 是名字与命令不同的函数。
var functionText = map[string]string{"limsup": "lim sup", "liminf": "lim inf"}

// spaces 是间距命令的宽度，单位 mu（18mu = 1em）。
var spaces = map[string]float32{
	",": 3, ":": 4, ">": 4, ";": 5, "!": -3, " ": 6,
	"thinspace": 3, "medspace": 4, "thickspace": 5, "negthinspace": -3,
	"enspace": 9, "quad": 18, "qquad": 36,
}

type accent struct {
	r     rune // 组合用重音字符；0 表示画横线
	wide  bool // 随内容加宽
	under bool // 横线画在下方
}

var accents = map[string]accent{
	"hat": {r: 0x0302}, "widehat": {r: 0x0302, wide: true},
	"tilde": {r: 0x0303}, "widetilde": {r: 0x0303, wide: true},
	"bar": {r: 0x0304}, "vec": {r: 0x20D7},
	"dot": {r: 0x0307}, "ddot": {r: 0x0308},
	"check": {r: 0x030C}, "breve": {r: 0x0306},
	"acute": {r: 0x0301}, "grave": {r: 0x0300}, "mathring": {r: 0x030A},
	"overrightarrow": {r: 0x20D7, wide: true}, "overleftarrow": {r: 0x20D6, wide: true},
	"overline": {}, "underline": {under: true},
}

// delimiters 是可以跟在 \left、\right、\big 之后的定界符；"." 表示空定界符。
var delimiters = map[string]rune{
	"(": '(', ")": ')', "[": '[', "]": ']', "|": '|', "/": '/', ".": 0,
	"<": 0x27E8, ">": 0x27E9,
	`\{`: '{', `\}`: '}', `\lbrace`: '{', `\rbrace`: '}',
	`\|`: 0x2016, `\Vert`: 0x2016, `\lVert`: 0x2016, `\rVert`: 0x2016,
	`\vert`: '|', `\lvert`: '|', `\rvert`: '|',
	`\langle`: 0x27E8, `\rangle`: 0x27E9,
	`\lfloor`: 0x230A, `\rfloor`: 0x230B, `\lceil`: 0x2308, `\rceil`: 0x2309,
	`\backslash`: '\\',
}

// bigSizes 是 \big 系列定界符的总高度，单位 em。
var bigSizes = map[string]float32{"big": 1.2, "Big": 1.8, "bigg": 2.4, "Bigg": 3.0}

// variantKind 是字母与数字使用的字体变体。
type variantKind uint8

const (
	varMath variantKind = iota // 默认：字母斜体，数字正体
	varRoman
	varBold
	varItalic
	varBoldItalic
	varBlackboard
	varScript
	varFraktur
)

var variantCommands = map[string]variantKind{
	"mathrm": varRoman, "mathbf": varBold, "mathit": varItalic,
	"mathbb": varBlackboard, "mathcal": varScript, "mathscr": varScript,
	"mathfrak": varFraktur, "boldsymbol": varBoldItalic, "bm": varBoldItalic,
	"mathsf": varRoman, "mathtt": varRoman, "mathnormal": varMath,
}

var textCommands = map[string]variantKind{
	"text": varRoman, "textrm": varRoman, "textnormal": varRoman, "mbox": varRoman,
	"textbf": varBold, "textit": varItalic,
}

// 数学字母区里被挪到字母式符号区的“空洞”。
var alphaHoles = map[rune]rune{
	0x1D455: 0x210E, // 斜体 h
	0x1D49D: 0x212C, 0x1D4A0: 0x2130, 0x1D4A1: 0x2131, 0x1D4A3: 0x210B,
	0x1D4A4: 0x2110, 0x1D4A7: 0x2112, 0x1D4A8: 0x2133, 0x1D4AD: 0x211B,
	0x1D4BA: 0x212F, 0x1D4BC: 0x210A, 0x1D4C4: 0x2134, // 花体
	0x1D506: 0x212D, 0x1D50B: 0x210C, 0x1D50C: 0x2111, 0x1D515: 0x211C,
	0x1D51D: 0x2128, // 哥特体
	0x1D53A: 0x2102, 0x1D53F: 0x210D, 0x1D545: 0x2115, 0x1D547: 0x2119,
	0x1D548: 0x211A, 0x1D549: 0x211D, 0x1D551: 0x2124, // 黑板粗体
}

// styled 把 ASCII 字母与数字映射到变体对应的 Unicode 数学字母；其余字符原样返回。
func styled(r rune, v variantKind) rune {
	var upper, lower, digit rune
	switch v {
	case varMath, varItalic:
		upper, lower = 0x1D434, 0x1D44E
	case varBold:
		upper, lower, digit = 0x1D400, 0x1D41A, 0x1D7CE
	case varBoldItalic:
		upper, lower, digit = 0x1D468, 0x1D482, 0x1D7CE
	case varBlackboard:
		upper, lower, digit = 0x1D538, 0x1D552, 0x1D7D8
	case varScript:
		upper, lower = 0x1D49C, 0x1D4B6
	case varFraktur:
		upper, lower = 0x1D504, 0x1D51E
	default:
		return r
	}
	out := r
	switch {
	case r >= 'A' && r <= 'Z' && upper != 0:
		out = upper + r - 'A'
	case r >= 'a' && r <= 'z' && lower != 0:
		out = lower + r - 'a'
	case r >= '0' && r <= '9' && digit != 0:
		out = digit + r - '0'
	}
	if hole, ok := alphaHoles[out]; ok {
		return hole
	}
	return out
}
