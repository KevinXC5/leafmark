package desktop

import (
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// nativeApplicationMenu 保留系统编辑、窗口和退出操作，文档关闭交给原生快捷键处理。
// 默认 RoleFileMenu 的 Command+W 会先关闭整窗，绕过当前标签的关闭命令。
func nativeApplicationMenu(app *nativeApp) *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
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
var shortcutActions = []struct{ id, label, command string }{
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
	shortcutPlainKey    = regexp.MustCompile(`^(?i)[a-z0-9,./;=\[\]\\]$`)
	shortcutFunctionKey = regexp.MustCompile(`^(?i)F([1-9]|1[0-2])$`)
)

// validateShortcut 把组合键整理成规范写法；拒绝没有修饰键的按键与系统、基础编辑保留的组合。
func validateShortcut(value string) (string, string) {
	parts := strings.Split(strings.TrimSpace(value), "-")
	key := parts[len(parts)-1]
	aliases := map[string]string{"mod": "Mod", "ctrl": "Ctrl", "control": "Ctrl", "meta": "Meta", "cmd": "Meta", "alt": "Alt", "shift": "Shift"}
	var mods []string
	for _, part := range parts[:len(parts)-1] {
		mod, ok := aliases[strings.ToLower(part)]
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
	for _, mod := range []string{"Mod", "Ctrl", "Meta", "Alt", "Shift"} {
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
	for _, mod := range []string{"Ctrl", "Meta", "Alt", "Shift"} {
		if slices.Contains(mods, mod) {
			ordered = append(ordered, mod)
		}
	}
	return strings.Join(append(ordered, key), "-")
}

// shortcutConflict 返回与 shortcut 冲突的另一个动作。两个平台都检查，避免换平台后重合。
func shortcutConflict(bindings nativeShortcuts, action, shortcut string) string {
	for _, other := range shortcutActions {
		if other.id == action {
			continue
		}
		for _, mac := range []bool{false, true} {
			if shortcutSignature(*bindings.binding(other.id), mac) == shortcutSignature(shortcut, mac) {
				return other.id
			}
		}
	}
	return ""
}

// checkedShortcuts 逐项校验绑定；有任何一项无效或互相冲突时整体回落默认。
func checkedShortcuts(bindings nativeShortcuts) nativeShortcuts {
	for _, item := range shortcutActions {
		field := bindings.binding(item.id)
		normal, problem := validateShortcut(*field)
		if problem != "" {
			return defaultNativeShortcuts()
		}
		*field = normal
	}
	for _, item := range shortcutActions {
		if shortcutConflict(bindings, item.id, *bindings.binding(item.id)) != "" {
			return defaultNativeShortcuts()
		}
	}
	return bindings
}

var shortcutPunctuation = map[string]ui.Key{
	",": ui.KeyComma, ".": ui.KeyPeriod, "/": ui.KeySlash, ";": ui.KeySemicolon, "=": ui.KeyEqual,
	"[": ui.KeyBracketLeft, "]": ui.KeyBracketRight, `\`: ui.KeyBackslash,
}

var shortcutFunctionKeys = []ui.Key{ui.KeyF1, ui.KeyF2, ui.KeyF3, ui.KeyF4, ui.KeyF5, ui.KeyF6, ui.KeyF7, ui.KeyF8, ui.KeyF9, ui.KeyF10, ui.KeyF11, ui.KeyF12}

// parseShortcut 把规范写法换成界面库的修饰键与按键。
func parseShortcut(shortcut string) (ui.Modifiers, ui.Key, bool) {
	parts := strings.Split(shortcut, "-")
	name := parts[len(parts)-1]
	var mods ui.Modifiers
	for _, part := range parts[:len(parts)-1] {
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
	}
	switch {
	case len(name) == 1 && name[0] >= 'a' && name[0] <= 'z':
		return mods, ui.KeyA + ui.Key(name[0]-'a'), true
	case len(name) == 1 && name[0] >= '0' && name[0] <= '9':
		return mods, ui.Key0 + ui.Key(name[0]-'0'), true
	case shortcutFunctionKey.MatchString(name):
		n, _ := strconv.Atoi(name[1:])
		return mods, shortcutFunctionKeys[n-1], true
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
