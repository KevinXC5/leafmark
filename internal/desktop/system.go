package desktop

import (
	"errors"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo"
)

func validateExternalURL(raw string) error {
	if len(raw) == 0 || len(raw) > 8192 || !utf8.ValidString(raw) {
		return errors.New("链接无效")
	}
	for _, r := range raw {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return errors.New("链接不能包含空白或控制字符")
		}
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return errors.New("链接格式无效")
	}
	// 仅允许明确的外部协议，禁止 file、javascript 和自定义应用协议。
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		if parsed.Hostname() == "" || parsed.Opaque != "" || parsed.User != nil {
			return errors.New("网页链接必须包含有效主机且不能包含凭据")
		}
	case "mailto", "tel":
		if parsed.Host != "" || (parsed.Opaque == "" && parsed.Path == "") {
			return errors.New("链接缺少邮件地址或电话号码")
		}
	default:
		return errors.New("仅允许 http、https、mailto 或 tel 链接")
	}
	return nil
}
func (w *Workspace) OpenExternal(raw string) error {
	if err := validateExternalURL(raw); err != nil {
		return err
	}
	return mygo.Shell.OpenExternal(raw)
}
func (w *Workspace) CopyText(text string) error {
	if len(text) > 16<<20 || !utf8.ValidString(text) || strings.ContainsRune(text, '\x00') {
		return errors.New("复制内容必须是 16 MB 以内的 UTF-8 文本")
	}
	mygo.Clipboard.WriteText(text)
	return nil
}
