package desktop

import (
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// nativeApplicationMenu 保留系统编辑、窗口和退出操作，文档关闭交给原生快捷键处理。
// 默认 RoleFileMenu 的 Command+W 会先关闭整窗，绕过当前标签的关闭命令。
func nativeApplicationMenu(app *nativeApp) *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu, Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleAbout},
			{ID: "settings", Label: "设置…", Click: func(_ *mygo.MenuItem, _ *mygo.Window) {
				// 菜单始终打开设置，快捷键仍由用户自定义绑定处理。
				app.settingsOn = true
				if win := app.window(); win != nil {
					win.Invalidate()
				}
			}},
			{ID: "check-updates", Label: "检查更新", Click: func(_ *mygo.MenuItem, _ *mygo.Window) {
				app.checkUpdate()
				if win := app.window(); win != nil {
					win.Invalidate()
				}
			}},
			mygo.Separator(),
			{Role: mygo.RoleServices},
			mygo.Separator(),
			{Role: mygo.RoleHide},
			{Role: mygo.RoleHideOthers},
			{Role: mygo.RoleUnhide},
			mygo.Separator(),
			{Role: mygo.RoleQuit},
		}},
		{Label: "文件", Submenu: []*mygo.MenuItem{
			{ID: "close-tab", Label: "关闭当前标签", Click: func(_ *mygo.MenuItem, win *mygo.Window) {
				if win != nil && win == app.window() && !app.settingsOn {
					app.command("close-tab")
					win.Invalidate()
				}
			}},
		}},
		{Role: mygo.RoleEditMenu},
		{Role: mygo.RoleViewMenu},
		{Role: mygo.RoleWindowMenu},
	})
}

// nativeShortcuts 是可自定义的快捷键绑定。写法如 "Mod-Shift-s"：
// Mod 在 macOS 是 ⌘，其他平台是 Ctrl。
type nativeShortcuts struct {
	Save     string `json:"save"`
	SaveAs   string `json:"saveAs"`
	Open     string `json:"open"`
	New      string `json:"new"`
	Close    string `json:"close"`
	Find     string `json:"find"`
	Bold     string `json:"bold"`
	Italic   string `json:"italic"`
	Link     string `json:"link"`
	Source   string `json:"source"`
	Settings string `json:"settings"`
}

func defaultNativeShortcuts() nativeShortcuts {
	return nativeShortcuts{
		Save: "Mod-s", SaveAs: "Mod-Shift-s", Open: "Mod-o", New: "Mod-n", Close: "Mod-w",
		Find: "Mod-f", Bold: "Mod-b", Italic: "Mod-i", Link: "Mod-k", Source: "Mod-Shift-r", Settings: "Mod-,",
	}
}

// shortcutActions 按设置页的显示顺序列出动作、名称与它触发的命令。
var shortcutActions = [...]struct{ id, label, command string }{
	{"save", "保存", "save"}, {"saveAs", "另存为", "save-as"}, {"open", "打开文件", "open"}, {"new", "新建文档", "new"},
	{"close", "关闭文档", "close-tab"}, {"find", "查找", "find"}, {"bold", "加粗", "bold"}, {"italic", "斜体", "italic"},
	{"link", "插入链接", "link"}, {"source", "切换源码模式", "source"}, {"settings", "设置", "settings"},
}

// binding 返回某个动作的绑定字段，未知动作返回 nil。
func (s *nativeShortcuts) binding(action string) *string {
	switch action {
	case "save":
		return &s.Save
	case "saveAs":
		return &s.SaveAs
	case "open":
		return &s.Open
	case "new":
		return &s.New
	case "close":
		return &s.Close
	case "find":
		return &s.Find
	case "bold":
		return &s.Bold
	case "italic":
		return &s.Italic
	case "link":
		return &s.Link
	case "source":
		return &s.Source
	case "settings":
		return &s.Settings
	}
	return nil
}

func shortcutLabel(action string) string {
	for _, item := range shortcutActions {
		if item.id == action {
			return item.label
		}
	}
	return action
}

var (
	shortcutPlainKey        = regexp.MustCompile(`^(?i)[a-z0-9,./;=\[\]\\]$`)
	shortcutFunctionKey     = regexp.MustCompile(`^(?i)F([1-9]|1[0-2])$`)
	shortcutModifierAliases = map[string]string{
		"mod":     "Mod",
		"ctrl":    "Ctrl",
		"control": "Ctrl",
		"meta":    "Meta",
		"cmd":     "Meta",
		"alt":     "Alt",
		"shift":   "Shift",
	}
	shortcutCanonicalMods = []string{"Mod", "Ctrl", "Meta", "Alt", "Shift"}
	shortcutPlatformMods  = []string{"Ctrl", "Meta", "Alt", "Shift"}
)

// validateShortcut 把组合键整理成规范写法；拒绝没有修饰键的按键与系统、基础编辑保留的组合。
func validateShortcut(value string) (string, string) {
	parts := strings.Split(strings.TrimSpace(value), "-")
	key := parts[len(parts)-1]
	var mods []string
	for _, part := range parts[:len(parts)-1] {
		mod, ok := shortcutModifierAliases[strings.ToLower(part)]
		if !ok || slices.Contains(mods, mod) {
			return "", "修饰键无效或重复，请使用 Mod、Ctrl、Meta、Alt、Shift。"
		}
		mods = append(mods, mod)
	}
	has := func(mod string) bool { return slices.Contains(mods, mod) }
	if has("Mod") && (has("Ctrl") || has("Meta")) {
		return "", "修饰键无效或重复，请使用 Mod、Ctrl、Meta、Alt、Shift。"
	}
	if !shortcutPlainKey.MatchString(key) && !shortcutFunctionKey.MatchString(key) {
		return "", "主键须为单个字母、数字、标点或 F1–F12。"
	}
	if shortcutFunctionKey.MatchString(key) {
		key = strings.ToUpper(key)
	} else {
		key = strings.ToLower(key)
	}
	primary := has("Mod") || has("Ctrl") || has("Meta")
	if !primary && !has("Alt") {
		return "", "请至少使用 Mod、Ctrl、Meta 或 Alt，避免影响正常输入。"
	}
	shift, alt := has("Shift"), has("Alt")
	in := func(keys ...string) bool { return slices.Contains(keys, key) }
	if (primary && in("q", "h", "m", "z", "x", "c", "v", "a")) ||
		(primary && shift && in("t", "n", "j", "i", "c")) ||
		(primary && !shift && in("r", "t", "l", "p", "j", "u")) ||
		(alt && key == "F4") || in("F5", "F11", "F12") {
		return "", "此组合键属于系统或基础编辑常用快捷键，请选择其他组合。"
	}
	var ordered []string
	for _, mod := range shortcutCanonicalMods {
		if has(mod) {
			ordered = append(ordered, mod)
		}
	}
	return strings.Join(append(ordered, key), "-"), ""
}

// shortcutSignature 把 Mod 换成某个平台的实际修饰键，用来比较两个绑定是否相同。
func shortcutSignature(shortcut string, mac bool) string {
	parts := strings.Split(shortcut, "-")
	key := parts[len(parts)-1]
	var mods []string
	for _, part := range parts[:len(parts)-1] {
		if part == "Mod" {
			part = "Ctrl"
			if mac {
				part = "Meta"
			}
		}
		mods = append(mods, part)
	}
	var ordered []string
	for _, mod := range shortcutPlatformMods {
		if slices.Contains(mods, mod) {
			ordered = append(ordered, mod)
		}
	}
	return strings.Join(append(ordered, key), "-")
}

// shortcutsOverlap 报告两个规范快捷键在任一平台上是否指向同一组合键。
func shortcutsOverlap(a, b string) bool {
	return shortcutSignature(a, false) == shortcutSignature(b, false) ||
		shortcutSignature(a, true) == shortcutSignature(b, true)
}

// shortcutConflict 返回与 shortcut 冲突的另一个动作。两个平台都检查，避免换平台后重合。
func shortcutConflict(bindings nativeShortcuts, action, shortcut string) string {
	for _, other := range shortcutActions {
		if other.id == action {
			continue
		}
		if shortcutsOverlap(*bindings.binding(other.id), shortcut) {
			return other.id
		}
	}
	return ""
}

// checkedShortcuts 逐项校验绑定：单条无效或与其他绑定冲突时，只将该项回退为默认快捷键，
// 保留其余合法且不冲突的自定义快捷键。
func checkedShortcuts(bindings nativeShortcuts) nativeShortcuts {
	defaults := defaultNativeShortcuts()
	var vals [len(shortcutActions)]string
	var defs [len(shortcutActions)]string
	var isDef [len(shortcutActions)]bool
	for i, item := range shortcutActions {
		defs[i] = *defaults.binding(item.id)
		normal, problem := validateShortcut(*bindings.binding(item.id))
		if problem != "" {
			vals[i] = defs[i]
			isDef[i] = true
			continue
		}
		vals[i] = normal
		isDef[i] = normal == defs[i]
	}
	// 若存在冲突：当一方为默认值（或因无效已回退默认）时回退非默认方，双方均为自定义时回退较后者。
	// 每轮至少多一项回到默认值，而默认集合内部两两不冲突，必在有限步内收敛。
	for {
		changed := false
		for i := 0; i < len(vals) && !changed; i++ {
			for j := i + 1; j < len(vals); j++ {
				if !shortcutsOverlap(vals[i], vals[j]) {
					continue
				}
				switch {
				case isDef[i] && !isDef[j]:
					vals[j], isDef[j] = defs[j], true
				case !isDef[i] && isDef[j]:
					vals[i], isDef[i] = defs[i], true
				default:
					vals[j], isDef[j] = defs[j], true
				}
				changed = true
				break
			}
		}
		if !changed {
			break
		}
	}
	out := defaults
	for i, item := range shortcutActions {
		*out.binding(item.id) = vals[i]
	}
	return out
}

var shortcutPunctuation = map[string]ui.Key{
	",": ui.KeyComma, ".": ui.KeyPeriod, "/": ui.KeySlash, ";": ui.KeySemicolon, "=": ui.KeyEqual,
	"[": ui.KeyBracketLeft, "]": ui.KeyBracketRight, `\`: ui.KeyBackslash,
}

var shortcutFunctionKeys = []ui.Key{ui.KeyF1, ui.KeyF2, ui.KeyF3, ui.KeyF4, ui.KeyF5, ui.KeyF6, ui.KeyF7, ui.KeyF8, ui.KeyF9, ui.KeyF10, ui.KeyF11, ui.KeyF12}

// parsedShortcut 缓存一条快捷键解析后的修饰键与按键，供热路径免分配检查。
type parsedShortcut struct {
	mods ui.Modifiers
	key  ui.Key
	ok   bool
}

var parsedShortcutsCache struct {
	mu       sync.Mutex
	ready    bool
	bindings nativeShortcuts
	parsed   [len(shortcutActions)]parsedShortcut
}

// parsedShortcuts 返回与 shortcutActions 顺序一致的预解析快捷键表，按 nativeShortcuts 值缓存。
func parsedShortcuts(bindings nativeShortcuts) [len(shortcutActions)]parsedShortcut {
	parsedShortcutsCache.mu.Lock()
	defer parsedShortcutsCache.mu.Unlock()
	if parsedShortcutsCache.ready && parsedShortcutsCache.bindings == bindings {
		return parsedShortcutsCache.parsed
	}
	var out [len(shortcutActions)]parsedShortcut
	for i, item := range shortcutActions {
		mods, key, ok := parseShortcut(*bindings.binding(item.id))
		out[i] = parsedShortcut{mods: mods, key: key, ok: ok}
	}
	parsedShortcutsCache.ready = true
	parsedShortcutsCache.bindings = bindings
	parsedShortcutsCache.parsed = out
	return out
}

func parseFunctionKeyNumber(digits string) (int, bool) {
	switch len(digits) {
	case 1:
		if digits[0] >= '1' && digits[0] <= '9' {
			return int(digits[0] - '0'), true
		}
	case 2:
		if digits[0] == '1' && digits[1] >= '0' && digits[1] <= '2' {
			return 10 + int(digits[1]-'0'), true
		}
	}
	return 0, false
}

// parseShortcut 把规范写法换成界面库的修饰键与按键；无切片分配、无正则匹配。
func parseShortcut(shortcut string) (ui.Modifiers, ui.Key, bool) {
	var mods ui.Modifiers
	rest := shortcut
	for {
		part, after, found := strings.Cut(rest, "-")
		if !found {
			break
		}
		switch part {
		case "Mod":
			mods |= ui.Cmd
		case "Ctrl":
			mods |= ui.Ctrl
		case "Meta":
			mods |= ui.Super
		case "Alt":
			mods |= ui.Alt
		case "Shift":
			mods |= ui.Shift
		default:
			return 0, 0, false
		}
		rest = after
	}
	name := rest
	switch {
	case len(name) == 1 && name[0] >= 'a' && name[0] <= 'z':
		return mods, ui.KeyA + ui.Key(name[0]-'a'), true
	case len(name) == 1 && name[0] >= '0' && name[0] <= '9':
		return mods, ui.Key0 + ui.Key(name[0]-'0'), true
	case len(name) >= 2 && len(name) <= 3 && (name[0] == 'F' || name[0] == 'f'):
		if n, ok := parseFunctionKeyNumber(name[1:]); ok {
			return mods, shortcutFunctionKeys[n-1], true
		}
	}
	key, ok := shortcutPunctuation[name]
	return mods, key, ok
}

// recordedShortcut 把一次按键写成绑定字符串；按下的只是修饰键或不支持的键时返回空串。
func recordedShortcut(mods ui.Modifiers, key ui.Key) string {
	name := ""
	switch {
	case key >= ui.KeyA && key <= ui.KeyZ:
		name = string(rune('a' + key - ui.KeyA))
	case key >= ui.Key0 && key <= ui.Key9:
		name = string(rune('0' + key - ui.Key0))
	default:
		for text, k := range shortcutPunctuation {
			if k == key {
				name = text
			}
		}
		if i := slices.Index(shortcutFunctionKeys, key); i >= 0 {
			name = "F" + strconv.Itoa(i+1)
		}
	}
	if name == "" {
		return ""
	}
	var parts []string
	mac := runtime.GOOS == "darwin"
	if mods&ui.Cmd != 0 {
		parts = append(parts, "Mod")
	}
	if mac && mods&ui.Ctrl != 0 {
		parts = append(parts, "Ctrl")
	}
	if !mac && mods&ui.Super != 0 {
		parts = append(parts, "Meta")
	}
	if mods&ui.Alt != 0 {
		parts = append(parts, "Alt")
	}
	if mods&ui.Shift != 0 {
		parts = append(parts, "Shift")
	}
	return strings.Join(append(parts, name), "-")
}
