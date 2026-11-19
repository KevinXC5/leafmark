package documents

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestUTF8RoundTripPreservesBOMAndLineEndings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "中文.md")
	original := "\ufeff# 标题\r\n\r\n你好 🌿\r\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	originalStat, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	s := NewStore("", "")
	doc, err := s.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Content != "# 标题\n\n你好 🌿\n" {
		t.Fatalf("读取结果：%q", doc.Content)
	}
	updated := doc.Content + "第二行\n"
	if err := s.Draft(doc.ID, updated); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(doc.ID, updated, path); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != original+"第二行\r\n" {
		t.Fatalf("保存结果：%q", raw)
	}
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if stat.Mode().Perm() != originalStat.Mode().Perm() {
		t.Fatal("保存改变了原文件权限")
	}
	if s.Current().Dirty {
		t.Fatal("保存后仍为未保存状态")
	}
}
func TestExternalChangesAreNotOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.md")
	os.WriteFile(path, []byte("原文"), 0644)
	s := NewStore("", "")
	doc, _ := s.Load(path)
	s.Draft(doc.ID, "我的修改")
	os.WriteFile(path, []byte("外部修改"), 0644)
	if _, err := s.Save(doc.ID, "我的修改", path); !errors.Is(err, ErrConflict) {
		t.Fatalf("预期冲突，得到 %v", err)
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != "外部修改" {
		t.Fatal("外部修改被覆盖")
	}
	if !s.Current().Dirty {
		t.Fatal("冲突后草稿状态丢失")
	}
}
func TestSaveKeepsNewerDraftAndRejectsOldDocument(t *testing.T) {
	s := NewStore("未命名.md", "")
	doc := s.Current()
	s.Draft(doc.ID, "新输入")
	path := filepath.Join(t.TempDir(), "note.md")
	saved, err := s.Save(doc.ID, "保存时的内容", path)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Content != "新输入" || !saved.Dirty {
		t.Fatal("保存覆盖了新的输入")
	}
	s.New("新文档.md", "")
	s.Close(doc.ID)
	if err := s.Draft(doc.ID, "旧输入"); !errors.Is(err, ErrStale) {
		t.Fatalf("预期过期文档错误：%v", err)
	}
}
func TestSymlinkSavePreservesLink(t *testing.T) {
	dir := t.TempDir()
	target, link := filepath.Join(dir, "target.md"), filepath.Join(dir, "link.md")
	os.WriteFile(target, []byte("原文"), 0644)
	if err := os.Symlink(target, link); err != nil {
		t.Skip(err)
	}
	s := NewStore("", "")
	doc, err := s.Load(link)
	if err != nil {
		t.Fatal(err)
	}
	s.Draft(doc.ID, "修改")
	if _, err := s.Save(doc.ID, "修改", doc.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Readlink(link); err != nil {
		t.Fatal("符号链接被替换")
	}
	raw, _ := os.ReadFile(target)
	if string(raw) != "修改" {
		t.Fatal("未更新链接目标")
	}
}

func TestSaveSnapshotKeepsSelectionEncodingAndNewerDraft(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "甲.md")
	if err := os.WriteFile(path, []byte("\ufeff原文\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s := NewStore("欢迎", "")
	first, err := s.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	second := s.New("乙", "乙")
	if err := s.Draft(first.ID, "新输入\n"); err != nil {
		t.Fatal(err)
	}
	saved, err := s.SaveSnapshot(first.ID, "快照\n", path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Current().ID != second.ID {
		t.Fatal("后台保存改变当前标签")
	}
	if saved.Content != "新输入\n" || !saved.Dirty {
		t.Fatalf("新草稿丢失：%+v", saved)
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != "\ufeff快照\r\n" {
		t.Fatalf("编码丢失：%q", raw)
	}
	if _, err := s.Save(first.ID, "旧接口", path); !errors.Is(err, ErrStale) {
		t.Fatalf("前台接口应拒绝非当前标签：%v", err)
	}
}

func TestConcurrentSnapshotSaveAndSaveAsRemainConsistent(t *testing.T) {
	s := NewStore("甲.md", "原文")
	id := s.Current().ID
	dir := t.TempDir()
	a, b := filepath.Join(dir, "甲.md"), filepath.Join(dir, "乙.md")
	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			path := a
			if i%2 == 1 {
				path = b
			}
			_, err := s.SaveSnapshot(id, "保存正文", path)
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	doc := s.Current()
	raw, err := os.ReadFile(doc.Path)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := s.SavedContent(id)
	if err != nil {
		t.Fatal(err)
	}
	if baseline != string(raw) {
		t.Fatalf("磁盘与保存基线不一致：%q %q", raw, baseline)
	}
	if state, err := s.CheckExternal(id); err != nil || state.Changed {
		t.Fatalf("并行保存产生虚假冲突：%+v %v", state, err)
	}
}

func TestReloadSnapshotRejectsNewDraftAndKeepsSelection(t *testing.T) {
	s := NewStore("甲", "")
	path := filepath.Join(t.TempDir(), "甲.md")
	os.WriteFile(path, []byte("磁盘"), 0600)
	doc, err := s.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	other := s.New("乙", "")
	_, raw, err := s.ReadSnapshot(doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	s.Draft(doc.ID, "新输入")
	if _, err := s.ReloadSnapshot(doc.ID, doc.Path, raw, false); !errors.Is(err, ErrDirty) {
		t.Fatalf("未保护新输入：%v", err)
	}
	if s.Current().ID != other.ID {
		t.Fatal("后台读取改变选择")
	}
}

func TestStoreGetCurrentBackgroundAndMissing(t *testing.T) {
	s := NewStore("甲.md", "初始甲")
	first := s.Current()
	if err := s.Draft(first.ID, "草稿甲"); err != nil {
		t.Fatal(err)
	}

	// 命中当前文档且反映最新草稿。
	got, ok := s.Get(first.ID)
	if !ok || got.ID != first.ID || got.Content != "草稿甲" || !got.Dirty {
		t.Fatalf("Get 当前文档结果错误：ok=%v doc=%+v", ok, got)
	}

	// 切换到新标签后，Get 仍能命中非当前会话文档且不改变当前选中标签。
	second := s.New("乙.md", "初始乙")
	bg, ok := s.Get(first.ID)
	if !ok || bg.ID != first.ID || bg.Content != "草稿甲" || !bg.Dirty {
		t.Fatalf("Get 非当前会话文档结果错误：ok=%v doc=%+v", ok, bg)
	}
	cur, ok := s.Get(second.ID)
	if !ok || cur.ID != second.ID || cur.Content != "初始乙" || cur.Dirty {
		t.Fatalf("Get 新当前文档结果错误：ok=%v doc=%+v", ok, cur)
	}
	if s.Current().ID != second.ID {
		t.Fatal("Get 不应改变当前选中标签")
	}

	// 不存在的 ID 与已关闭的 ID 均应返回 false。
	for _, id := range []string{"", "不存在的-id"} {
		if doc, ok := s.Get(id); ok || doc != (Document{}) {
			t.Fatalf("不存在 ID %q 应返回 false：ok=%v doc=%+v", id, ok, doc)
		}
	}
	if err := s.Close(first.ID); err != nil {
		t.Fatal(err)
	}
	if doc, ok := s.Get(first.ID); ok || doc != (Document{}) {
		t.Fatalf("已关闭标签 ID 应返回 false：ok=%v doc=%+v", ok, doc)
	}
}

func TestDecodeTextBOMAndCRLF(t *testing.T) {
	cases := []struct {
		name     string
		raw      []byte
		wantText string
		wantBOM  bool
		wantCRLF bool
		wantErr  bool
	}{
		{
			name:     "普通LF文本",
			raw:      []byte("# 标题\n正文\n"),
			wantText: "# 标题\n正文\n",
		},
		{
			name:     "仅BOM无CRLF",
			raw:      append([]byte{0xef, 0xbb, 0xbf}, []byte("# 中文\n行二\n")...),
			wantText: "# 中文\n行二\n",
			wantBOM:  true,
		},
		{
			name:     "仅CRLF无BOM",
			raw:      []byte("第一行\r\n第二行\r\n"),
			wantText: "第一行\n第二行\n",
			wantCRLF: true,
		},
		{
			name:     "同时含BOM和CRLF",
			raw:      append([]byte{0xef, 0xbb, 0xbf}, []byte("甲\r\n乙\r\n")...),
			wantText: "甲\n乙\n",
			wantBOM:  true,
			wantCRLF: true,
		},
		{
			name:    "非法UTF8拒绝",
			raw:     []byte{0xff, 0xfe},
			wantErr: true,
		},
		{
			name:    "含NUL字节拒绝",
			raw:     []byte("abc\x00def"),
			wantErr: true,
		},
		{
			name:    "超过大小上限拒绝",
			raw:     bytes.Repeat([]byte{'a'}, maxBytes+1),
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text, bom, crlf, err := decodeText(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatal("预期返回错误，实际为 nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("意外错误：%v", err)
			}
			if text != tc.wantText || bom != tc.wantBOM || crlf != tc.wantCRLF {
				t.Fatalf("decodeText 结果不匹配：got=(%q, bom=%v, crlf=%v), want=(%q, bom=%v, crlf=%v)",
					text, bom, crlf, tc.wantText, tc.wantBOM, tc.wantCRLF)
			}
		})
	}

	// 验证正文中不含 CRLF 时 decodeText 与字符串转换保持一致。
	if got, bom, crlf, err := decodeText([]byte(strings.Repeat("叶脉\n", 16))); err != nil || bom || crlf || len(got) == 0 {
		t.Fatalf("重复行解码异常：bom=%v crlf=%v err=%v", bom, crlf, err)
	}
}
