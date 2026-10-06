package mathlayout

import "sort"

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
	// 常见扩展符号与箭头
	"triangleleft": {0x25C1, clsBin}, "triangleright": {0x25B7, clsBin}, "diamond": {0x22C4, clsBin},
	"uplus": {0x228E, clsBin}, "sqcap": {0x2293, clsBin}, "sqcup": {0x2294, clsBin}, "amalg": {0x2A3F, clsBin},
	"oslash": {0x2298, clsBin}, "bigcirc": {0x25EF, clsBin}, "dagger": {0x2020, clsBin}, "ddagger": {0x2021, clsBin},
	"wr": {0x2240, clsBin}, "smallsetminus": {0x2216, clsBin},
	"prec": {0x227A, clsRel}, "succ": {0x227B, clsRel}, "preceq": {0x2AAF, clsRel}, "succeq": {0x2AB0, clsRel},
	"doteq": {0x2250, clsRel}, "asymp": {0x224D, clsRel}, "bowtie": {0x22C8, clsRel}, "smile": {0x2323, clsRel}, "frown": {0x2322, clsRel},
	"sqsubseteq": {0x2291, clsRel}, "sqsupseteq": {0x2292, clsRel}, "subsetneq": {0x228A, clsRel}, "supsetneq": {0x228B, clsRel},
	"nleq": {0x2270, clsRel}, "ngeq": {0x2271, clsRel}, "nsubseteq": {0x2288, clsRel}, "nsupseteq": {0x2289, clsRel},
	"nmid": {0x2224, clsRel}, "nparallel": {0x2226, clsRel},
	"hookrightarrow": {0x21AA, clsRel}, "hookleftarrow": {0x21A9, clsRel}, "rightleftharpoons": {0x21CC, clsRel},
	"leftharpoonup": {0x21BC, clsRel}, "leftharpoondown": {0x21BD, clsRel}, "rightharpoonup": {0x21C0, clsRel}, "rightharpoondown": {0x21C1, clsRel},
	"longleftrightarrow": {0x27F7, clsRel}, "Longrightarrow": {0x27F9, clsRel}, "Longleftarrow": {0x27F8, clsRel}, "Longleftrightarrow": {0x27FA, clsRel},
	"longmapsto": {0x27FC, clsRel}, "nearrow": {0x2197, clsRel}, "searrow": {0x2198, clsRel}, "swarrow": {0x2199, clsRel}, "nwarrow": {0x2196, clsRel},
	"Uparrow": {0x21D1, clsRel}, "Downarrow": {0x21D3, clsRel}, "updownarrow": {0x2195, clsRel}, "Updownarrow": {0x21D5, clsRel},
	"wp": {0x2118, clsOrd}, "mho": {0x2127, clsOrd}, "beth": {0x2136, clsOrd}, "gimel": {0x2137, clsOrd}, "daleth": {0x2138, clsOrd},
	"surd": {0x221A, clsOrd}, "clubsuit": {0x2663, clsOrd}, "diamondsuit": {0x2662, clsOrd}, "heartsuit": {0x2661, clsOrd}, "spadesuit": {0x2660, clsOrd},
	"checkmark": {0x2713, clsOrd}, "hslash": {0x210F, clsOrd}, "dotsb": {0x22EF, clsInner}, "dotsc": {0x2026, clsInner}, "dotsi": {0x22EF, clsInner}, "dotsm": {0x22EF, clsInner},
	// AMS 关系符
	"leqslant": {0x2A7D, clsRel}, "geqslant": {0x2A7E, clsRel}, "lesssim": {0x2272, clsRel}, "gtrsim": {0x2273, clsRel},
	"lessapprox": {0x2A85, clsRel}, "gtrapprox": {0x2A86, clsRel}, "lessgtr": {0x2276, clsRel}, "gtrless": {0x2277, clsRel},
	"nless": {0x226E, clsRel}, "ngtr": {0x226F, clsRel}, "lneq": {0x2A87, clsRel}, "gneq": {0x2A88, clsRel},
	"leqq": {0x2266, clsRel}, "geqq": {0x2267, clsRel}, "lll": {0x22D8, clsRel}, "ggg": {0x22D9, clsRel},
	"nsim": {0x2241, clsRel}, "ncong": {0x2247, clsRel}, "approxeq": {0x224A, clsRel}, "triangleq": {0x225C, clsRel},
	"coloneqq": {0x2254, clsRel}, "coloneq": {0x2254, clsRel}, "eqqcolon": {0x2255, clsRel}, "eqcirc": {0x2256, clsRel},
	"circeq": {0x2257, clsRel}, "bumpeq": {0x224F, clsRel}, "backsim": {0x223D, clsRel}, "eqsim": {0x2242, clsRel},
	"thicksim": {0x223C, clsRel}, "thickapprox": {0x2248, clsRel}, "varpropto": {0x221D, clsRel},
	"sqsubset": {0x228F, clsRel}, "sqsupset": {0x2290, clsRel}, "Subset": {0x22D0, clsRel}, "Supset": {0x22D1, clsRel},
	"subseteqq": {0x2AC5, clsRel}, "supseteqq": {0x2AC6, clsRel}, "subsetneqq": {0x2ACB, clsRel}, "supsetneqq": {0x2ACC, clsRel},
	"nprec": {0x2280, clsRel}, "nsucc": {0x2281, clsRel}, "precsim": {0x227E, clsRel}, "succsim": {0x227F, clsRel},
	"preccurlyeq": {0x227C, clsRel}, "succcurlyeq": {0x227D, clsRel},
	"vDash": {0x22A8, clsRel}, "Vdash": {0x22A9, clsRel}, "dashv": {0x22A3, clsRel}, "nvdash": {0x22AC, clsRel},
	"nvDash": {0x22AD, clsRel}, "Vvdash": {0x22AA, clsRel}, "owns": {0x220B, clsRel}, "between": {0x226C, clsRel},
	"pitchfork": {0x22D4, clsRel}, "multimap": {0x22B8, clsRel}, "shortmid": {0x2223, clsRel}, "shortparallel": {0x2225, clsRel},
	"vartriangleleft": {0x22B2, clsRel}, "vartriangleright": {0x22B3, clsRel}, "trianglelefteq": {0x22B4, clsRel}, "trianglerighteq": {0x22B5, clsRel},
	// AMS 箭头
	"nrightarrow": {0x219B, clsRel}, "nleftarrow": {0x219A, clsRel}, "nRightarrow": {0x21CF, clsRel}, "nLeftarrow": {0x21CD, clsRel},
	"nleftrightarrow": {0x21AE, clsRel}, "nLeftrightarrow": {0x21CE, clsRel}, "leadsto": {0x21DD, clsRel}, "rightsquigarrow": {0x21DD, clsRel},
	"leftrightsquigarrow": {0x21AD, clsRel}, "twoheadrightarrow": {0x21A0, clsRel}, "twoheadleftarrow": {0x219E, clsRel},
	"rightarrowtail": {0x21A3, clsRel}, "leftarrowtail": {0x21A2, clsRel}, "leftrightarrows": {0x21C6, clsRel}, "rightleftarrows": {0x21C4, clsRel},
	"rightrightarrows": {0x21C9, clsRel}, "leftleftarrows": {0x21C7, clsRel}, "leftrightharpoons": {0x21CB, clsRel},
	"upharpoonright": {0x21BE, clsRel}, "restriction": {0x21BE, clsRel}, "upharpoonleft": {0x21BF, clsRel},
	"downharpoonright": {0x21C2, clsRel}, "downharpoonleft": {0x21C3, clsRel}, "curvearrowright": {0x21B7, clsRel}, "curvearrowleft": {0x21B6, clsRel},
	"circlearrowright": {0x21BB, clsRel}, "circlearrowleft": {0x21BA, clsRel}, "Lleftarrow": {0x21DA, clsRel}, "Rrightarrow": {0x21DB, clsRel},
	"impliedby": {0x27F8, clsRel}, "dashrightarrow": {0x21E2, clsRel}, "dashleftarrow": {0x21E0, clsRel},
	"looparrowright": {0x21AC, clsRel}, "looparrowleft": {0x21AB, clsRel}, "Lsh": {0x21B0, clsRel}, "Rsh": {0x21B1, clsRel},
	// AMS 二元运算符
	"ltimes": {0x22C9, clsBin}, "rtimes": {0x22CA, clsBin}, "boxplus": {0x229E, clsBin}, "boxminus": {0x229F, clsBin},
	"boxtimes": {0x22A0, clsBin}, "boxdot": {0x22A1, clsBin}, "circledast": {0x229B, clsBin}, "circledcirc": {0x229A, clsBin},
	"circleddash": {0x229D, clsBin}, "dotplus": {0x2214, clsBin}, "divideontimes": {0x22C7, clsBin}, "Cup": {0x22D3, clsBin},
	"Cap": {0x22D2, clsBin}, "doublecup": {0x22D3, clsBin}, "doublecap": {0x22D2, clsBin}, "curlyvee": {0x22CE, clsBin},
	"curlywedge": {0x22CF, clsBin}, "veebar": {0x22BB, clsBin}, "barwedge": {0x22BC, clsBin}, "intercal": {0x22BA, clsBin},
	"centerdot": {0x22C5, clsBin}, "bigtriangleup": {0x25B3, clsBin}, "bigtriangledown": {0x25BD, clsBin},
	"lhd": {0x22B2, clsBin}, "rhd": {0x22B3, clsBin}, "unlhd": {0x22B4, clsBin}, "unrhd": {0x22B5, clsBin},
	"leftthreetimes": {0x22CB, clsBin}, "rightthreetimes": {0x22CC, clsBin},
	// 其他普通符号
	"Box": {0x25A1, clsOrd}, "Diamond": {0x25C7, clsOrd}, "blacksquare": {0x25A0, clsOrd}, "blacktriangle": {0x25B4, clsOrd},
	"blacktriangledown": {0x25BE, clsOrd}, "bigstar": {0x2605, clsOrd}, "lozenge": {0x25CA, clsOrd}, "blacklozenge": {0x29EB, clsOrd},
	"triangledown": {0x25BD, clsOrd}, "vartriangle": {0x25B3, clsOrd}, "complement": {0x2201, clsOrd}, "eth": {0x00F0, clsOrd},
	"digamma": {0x03DD, clsOrd}, "Finv": {0x2132, clsOrd}, "Game": {0x2141, clsOrd}, "Bbbk": {0x1D55C, clsOrd},
	"flat": {0x266D, clsOrd}, "natural": {0x266E, clsOrd}, "sharp": {0x266F, clsOrd}, "measuredangle": {0x2221, clsOrd},
	"sphericalangle": {0x2222, clsOrd}, "backprime": {0x2035, clsOrd}, "diagup": {0x2571, clsOrd}, "diagdown": {0x2572, clsOrd},
	"maltese": {0x2720, clsOrd}, "circledS": {0x24C8, clsOrd}, "circledR": {0x00AE, clsOrd}, "yen": {0x00A5, clsOrd},
	"pounds": {0x00A3, clsOrd}, "S": {0x00A7, clsOrd}, "P": {0x00B6, clsOrd}, "dag": {0x2020, clsOrd}, "ddag": {0x2021, clsOrd},
	"iddots": {0x22F0, clsInner}, "cdotp": {0x22C5, clsPunct}, "ldotp": {'.', clsPunct},
	// 斜体大写希腊字母
	"varGamma": {0x1D6E4, clsOrd}, "varDelta": {0x1D6E5, clsOrd}, "varTheta": {0x1D6E9, clsOrd}, "varLambda": {0x1D6EC, clsOrd},
	"varXi": {0x1D6EF, clsOrd}, "varPi": {0x1D6F1, clsOrd}, "varSigma": {0x1D6F4, clsOrd}, "varUpsilon": {0x1D6F6, clsOrd},
	"varPhi": {0x1D6F7, clsOrd}, "varPsi": {0x1D6F9, clsOrd}, "varOmega": {0x1D6FA, clsOrd},
	// 定界符与标点
	"lbrack": {'[', clsOpen}, "rbrack": {']', clsClose}, "lparen": {'(', clsOpen}, "rparen": {')', clsClose},
	"lgroup": {0x27EE, clsOpen}, "rgroup": {0x27EF, clsClose}, "llbracket": {0x27E6, clsOpen}, "rrbracket": {0x27E7, clsClose},
	"ulcorner": {0x231C, clsOpen}, "urcorner": {0x231D, clsClose}, "llcorner": {0x231E, clsOpen}, "lrcorner": {0x231F, clsClose},
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
	// 同一个字符有多个命令时按命令名排序取第一个，结果才不随 map 的遍历顺序变化。
	names := make([]string, 0, len(symbols))
	for name := range symbols {
		names = append(names, name)
	}
	sort.Strings(names)
	m := map[rune]class{}
	for _, name := range names {
		s := symbols[name]
		if _, seen := m[s.r]; !seen && s.r > 0x7F {
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
	"oint": {0x222E, false}, "oiint": {0x222F, false}, "oiiint": {0x2230, false}, "iiiint": {0x2A0C, false},
	"intop": {0x222B, true}, "bigsqcap": {0x2A05, true},
}

// functions 是正体函数名；值为 true 的在块级样式下把下标放到正下方。
var functions = map[string]bool{
	"sin": false, "cos": false, "tan": false, "cot": false, "sec": false, "csc": false,
	"arcsin": false, "arccos": false, "arctan": false,
	"sinh": false, "cosh": false, "tanh": false, "coth": false,
	"log": false, "ln": false, "lg": false, "exp": false,
	"arg": false, "deg": false, "dim": false, "hom": false, "ker": false,
	"lim": true, "limsup": true, "liminf": true, "max": true, "min": true,
	"sech": false, "csch": false, "arccot": false, "arccsc": false, "arcsec": false, "injlim": true, "projlim": true,
	"sup": true, "inf": true, "det": true, "gcd": true, "Pr": true,
	"argmax": true, "argmin": true, "plim": true,
	"tg": false, "ctg": false, "cotg": false, "arctg": false, "arcctg": false, "cosec": false,
	"sh": false, "ch": false, "th": false, "cth": false,
}

// functionText 是名字与命令不同的函数。
var functionText = map[string]string{"limsup": "lim sup", "liminf": "lim inf", "injlim": "inj lim", "projlim": "proj lim", "argmax": "arg max", "argmin": "arg min"}

// spaces 是间距命令的宽度，单位 mu（18mu = 1em）。
var spaces = map[string]float32{
	",": 3, ":": 4, ">": 4, ";": 5, "!": -3, " ": 6,
	"thinspace": 3, "medspace": 4, "thickspace": 5, "negthinspace": -3,
	"negmedspace": -4, "negthickspace": -5, "enskip": 9,
	"enspace": 9, "quad": 18, "qquad": 36,
	"space": 6, "nobreakspace": 6, "hfill": 0, "allowbreak": 0, "nobreak": 0, "relax": 0,
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
	"dot": {r: 0x0307}, "ddot": {r: 0x0308}, "dddot": {r: 0x20DB}, "ddddot": {r: 0x20DC},
	"widecheck": {r: 0x030C, wide: true}, "overleftrightarrow": {r: 0x20E1, wide: true},
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
	`\lbrack`:    '[', `\rbrack`: ']', `\lparen`: '(', `\rparen`: ')',
	`\lgroup`: 0x27EE, `\rgroup`: 0x27EF, `\llbracket`: 0x27E6, `\rrbracket`: 0x27E7,
	`\ulcorner`: 0x231C, `\urcorner`: 0x231D, `\llcorner`: 0x231E, `\lrcorner`: 0x231F,
	`\uparrow`: 0x2191, `\downarrow`: 0x2193, `\updownarrow`: 0x2195,
	`\Uparrow`: 0x21D1, `\Downarrow`: 0x21D3, `\Updownarrow`: 0x21D5,
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
	varSans
	varMono
)

var variantCommands = map[string]variantKind{
	"mathrm": varRoman, "mathbf": varBold, "mathit": varItalic,
	"mathbb": varBlackboard, "mathcal": varScript, "mathscr": varScript,
	"mathfrak": varFraktur, "boldsymbol": varBoldItalic, "bm": varBoldItalic,
	"mathsf": varSans, "mathtt": varMono, "mathnormal": varMath,
	"pmb": varBoldItalic, "boldmath": varBoldItalic, "bold": varBold, "Bbb": varBlackboard, "frak": varFraktur,
}

// variantSwitches 是旧式的字体切换命令，作用到所在组的结尾。
var variantSwitches = map[string]variantKind{
	"rm": varRoman, "bf": varBold, "it": varItalic, "cal": varScript, "sf": varSans, "tt": varMono, "mit": varMath,
}

var textCommands = map[string]variantKind{
	"text": varRoman, "textrm": varRoman, "textnormal": varRoman, "mbox": varRoman,
	"textbf": varBold, "textit": varItalic, "emph": varItalic, "textsl": varItalic,
	"textsf": varSans, "texttt": varMono, "textup": varRoman, "textmd": varRoman, "hbox": varRoman,
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
	case varSans:
		upper, lower, digit = 0x1D5A0, 0x1D5BA, 0x1D7E2
	case varMono:
		upper, lower, digit = 0x1D670, 0x1D68A, 0x1D7F6
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

// styledSymbol 让希腊字母与 ∇ 跟随粗体、斜体和正体变体；其余符号原样返回。
func styledSymbol(r rune, v variantKind) rune {
	const (
		italicUpper, italicLower = 0x1D6E2, 0x1D6FC // 斜体区的 Α 与 α
		boldShift                = 0x1D6A8 - italicUpper
		boldItalicShift          = 0x1D71C - italicUpper
	)
	// 先归一到数学斜体区里的位置，再按变体平移。
	at := rune(0)
	switch {
	case r >= 0x0391 && r <= 0x03A9:
		at = italicUpper + r - 0x0391
	case r == 0x2207:
		at = 0x1D6FB
	case r >= italicLower && r <= 0x1D71B:
		at = r
	default:
		return r
	}
	switch v {
	case varBold:
		return at + boldShift
	case varBoldItalic:
		return at + boldItalicShift
	case varItalic:
		return at
	case varRoman:
		if r >= italicLower && r <= 0x1D714 {
			return 0x03B1 + r - italicLower
		}
	}
	return r
}

// negated 是 \not 后面有现成否定字形的关系符。
var negated = map[rune]rune{
	'=': 0x2260, '<': 0x226E, '>': 0x226F, 0x2264: 0x2270, 0x2265: 0x2271, 0x2261: 0x2262,
	0x2208: 0x2209, 0x220B: 0x220C, 0x2282: 0x2284, 0x2283: 0x2285, 0x2286: 0x2288, 0x2287: 0x2289,
	0x223C: 0x2241, 0x2243: 0x2244, 0x2245: 0x2247, 0x2248: 0x2249, 0x2223: 0x2224, 0x2225: 0x2226,
	0x2203: 0x2204, 0x227A: 0x2280, 0x227B: 0x2281, 0x22A2: 0x22AC, 0x22A8: 0x22AD,
	0x2192: 0x219B, 0x2190: 0x219A, 0x21D2: 0x21CF, 0x21D0: 0x21CD, 0x2194: 0x21AE, 0x21D4: 0x21CE,
}

// namedColors 是 \color 与 \textcolor 认识的颜色名，取值与 xcolor 的基础色一致。
var namedColors = map[string][3]uint8{
	"black": {0, 0, 0}, "white": {255, 255, 255}, "red": {255, 0, 0}, "green": {0, 255, 0}, "blue": {0, 0, 255},
	"cyan": {0, 255, 255}, "magenta": {255, 0, 255}, "yellow": {255, 255, 0}, "orange": {255, 128, 0},
	"purple": {191, 0, 64}, "violet": {128, 0, 128}, "brown": {191, 128, 64}, "pink": {255, 191, 191},
	"gray": {128, 128, 128}, "grey": {128, 128, 128}, "lightgray": {191, 191, 191}, "darkgray": {64, 64, 64},
	"teal": {0, 128, 128}, "olive": {128, 128, 0}, "lime": {191, 255, 0},
}
