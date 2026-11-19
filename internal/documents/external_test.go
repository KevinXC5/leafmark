package documents

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func loadExternalTestDocument(t *testing.T, s *Store, raw string) Document {
	t.Helper()
	path := filepath.Join(t.TempDir(), "文档.md")
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	doc, err := s.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func writeExternalTestDocument(t *testing.T, path, raw string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestCheckExternalUsesSavedDigestWithoutChangingSelection(t *testing.T) {
	s := NewStore("未命名.md", "")
	doc := loadExternalTestDocument(t, s, "原文\n")
	if err := s.Draft(doc.ID, "草稿"); err != nil {
		t.Fatal(err)
	}
	selected := s.New("当前.md", "当前草稿")
	state, err := s.CheckExternal(doc.ID)
	if err != nil || state.Changed || state.Missing {
		t.Fatalf("草稿不应被识别为磁盘变化：%+v %v", state, err)
	}
	// 规范化后的内容相同，原始换行和 BOM 的改变仍应影响摘要。
	writeExternalTestDocument(t, doc.Path, string(rune(0xfeff))+"原文\r\n")
	for range 2 {
		state, err = s.CheckExternal(doc.ID)
		if err != nil || !state.Changed || state.Missing {
			t.Fatalf("预期磁盘变化：%+v %v", state, err)
		}
	}
	if s.Current() != selected {
		t.Fatal("检查改变了当前标签")
	}
	opened, err := s.Select(doc.ID)
	if err != nil || opened.Content != "草稿" || !opened.Dirty {
		t.Fatalf("检查改变了草稿：%+v %v", opened, err)
	}
}

func TestReloadRequiresDiscardAndPreservesIDAndSelection(t *testing.T) {
	s := NewStore("", "")
	doc := loadExternalTestDocument(t, s, "原文")
	if err := s.Draft(doc.ID, "草稿"); err != nil {
		t.Fatal(err)
	}
	selected := s.New("当前.md", "当前草稿")
	writeExternalTestDocument(t, doc.Path, string(rune(0xfeff))+"外部内容\r\n")
	if _, err := s.Reload(doc.ID, false); !errors.Is(err, ErrDirty) {
		t.Fatalf("预期拒绝丢弃草稿：%v", err)
	}
	entry := s.sessions[doc.ID]
	if entry.doc.Content != "草稿" || !entry.doc.Dirty {
		t.Fatal("拒绝后草稿丢失")
	}
	reloaded, err := s.Reload(doc.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.ID != doc.ID || reloaded.Path != doc.Path || reloaded.Name != doc.Name || reloaded.Content != "外部内容\n" || reloaded.Dirty {
		t.Fatalf("重新加载结果错误：%+v", reloaded)
	}
	if s.Current() != selected {
		t.Fatal("重新加载改变了当前标签")
	}
	state, err := s.CheckExternal(doc.ID)
	if err != nil || state.Changed || state.Missing {
		t.Fatalf("重新加载未更新摘要：%+v %v", state, err)
	}
	if err := s.Draft(doc.ID, reloaded.Content); err != nil || s.sessions[doc.ID].doc.Dirty {
		t.Fatal("重新加载未更新保存基线")
	}
	opened, err := s.Load(doc.Path)
	if err != nil || opened.ID != doc.ID {
		t.Fatalf("重复打开创建了新标签：%+v %v", opened, err)
	}
	count := 0
	for _, item := range s.List() {
		if item.Path == doc.Path {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("重复路径标签数量：%d", count)
	}
	if _, err := s.Save(doc.ID, "更新\n", doc.Path); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(doc.Path)
	if err != nil || string(raw) != string(rune(0xfeff))+"更新\r\n" {
		t.Fatalf("重新加载未更新 BOM/CRLF：%q %v", raw, err)
	}
	writeExternalTestDocument(t, doc.Path, "普通内容\n")
	if _, err := s.Reload(doc.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(doc.ID, "再次更新\n", doc.Path); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(doc.Path)
	if err != nil || string(raw) != "再次更新\n" {
		t.Fatalf("重新加载未清除 BOM/CRLF：%q %v", raw, err)
	}
}

func TestExternalMissingAfterRenameKeepsDraft(t *testing.T) {
	s := NewStore("", "")
	doc := loadExternalTestDocument(t, s, "原文")
	s.Draft(doc.ID, "草稿")
	before := s.Current()
	if err := os.Rename(doc.Path, doc.Path+".renamed"); err != nil {
		t.Fatal(err)
	}
	state, err := s.CheckExternal(doc.ID)
	if err != nil || !state.Missing || state.Changed {
		t.Fatalf("预期原路径缺失：%+v %v", state, err)
	}
	if _, err := s.Reload(doc.ID, true); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("预期文件不存在：%v", err)
	}
	if s.Current() != before {
		t.Fatal("文件缺失后草稿变化")
	}
}

func TestExternalUntitledAndUnknownID(t *testing.T) {
	s := NewStore("未命名.md", "草稿")
	doc := s.Current()
	state, err := s.CheckExternal(doc.ID)
	if err != nil || state != (ExternalState{}) {
		t.Fatalf("无路径文档状态错误：%+v %v", state, err)
	}
	if _, err := s.Reload(doc.ID, true); err == nil {
		t.Fatal("无路径文档不应允许重新加载")
	}
	for _, id := range []string{"", "未知 id"} {
		if _, err := s.CheckExternal(id); !errors.Is(err, ErrStale) {
			t.Fatalf("未知 id 检查结果：%v", err)
		}
		if _, err := s.Reload(id, true); !errors.Is(err, ErrStale) {
			t.Fatalf("未知 id 重载结果：%v", err)
		}
	}
	s.Close(doc.ID)
	if _, err := s.CheckExternal(doc.ID); !errors.Is(err, ErrStale) {
		t.Fatalf("关闭的 id 不应有效：%v", err)
	}
	if _, err := s.Reload(doc.ID, true); !errors.Is(err, ErrStale) {
		t.Fatalf("关闭的 id 不应重新加载：%v", err)
	}
}

func TestExternalInvalidContentAndSizeKeepSnapshot(t *testing.T) {
	for _, raw := range []string{"\xff", "包含\x00内容", strings.Repeat("x", maxBytes+1)} {
		t.Run("无效内容", func(t *testing.T) {
			s := NewStore("", "")
			doc := loadExternalTestDocument(t, s, "原文")
			s.Draft(doc.ID, "草稿")
			before := s.Current()
			writeExternalTestDocument(t, doc.Path, raw)
			if len(raw) <= maxBytes {
				state, err := s.CheckExternal(doc.ID)
				if err != nil || !state.Changed {
					t.Fatalf("应检测到无效文本的字节变化：%+v %v", state, err)
				}
			} else if _, err := s.CheckExternal(doc.ID); err == nil {
				t.Fatal("检查未限制文件大小")
			}
			if _, err := s.Reload(doc.ID, true); err == nil {
				t.Fatal("不应加载无效内容")
			}
			if s.Current() != before {
				t.Fatal("失败后草稿变化")
			}
			writeExternalTestDocument(t, doc.Path, "原文")
			state, err := s.CheckExternal(doc.ID)
			if err != nil || state.Changed {
				t.Fatalf("失败后摘要变化：%+v %v", state, err)
			}
		})
	}
	s := NewStore("", "")
	doc := loadExternalTestDocument(t, s, "原文")
	writeExternalTestDocument(t, doc.Path, strings.Repeat("x", maxBytes))
	if state, err := s.CheckExternal(doc.ID); err != nil || !state.Changed {
		t.Fatalf("应允许恰好 16 MB：%+v %v", state, err)
	}
	if reloaded, err := s.Reload(doc.ID, false); err != nil || len(reloaded.Content) != maxBytes {
		t.Fatalf("应允许重载恰好 16 MB：%v", err)
	}
}

func TestExternalRejectsReplacedSymlinksAndDirectories(t *testing.T) {
	for _, replacement := range []string{"文件链接", "悬空链接", "父目录链接", "目录"} {
		t.Run(replacement, func(t *testing.T) {
			s := NewStore("", "")
			doc := loadExternalTestDocument(t, s, "原文")
			before := s.Current()
			outside := filepath.Join(t.TempDir(), "文档.md")
			writeExternalTestDocument(t, outside, "未授权内容")
			if replacement == "父目录链接" {
				dir := filepath.Dir(doc.Path)
				if err := os.Rename(dir, dir+".old"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Dir(outside), dir); err != nil {
					t.Skip(err)
				}
			} else {
				if err := os.Remove(doc.Path); err != nil {
					t.Fatal(err)
				}
				if replacement == "目录" {
					if err := os.Mkdir(doc.Path, 0700); err != nil {
						t.Fatal(err)
					}
				} else {
					if replacement == "悬空链接" {
						outside += ".missing"
					}
					if err := os.Symlink(outside, doc.Path); err != nil {
						t.Skip(err)
					}
				}
			}
			state, err := s.CheckExternal(doc.ID)
			if err == nil && !state.Missing {
				t.Fatal("检查不应接受替换后的链接或目录")
			}
			if _, err := s.Reload(doc.ID, true); err == nil {
				t.Fatal("不应重新加载替换后的链接或目录")
			}
			if s.Current() != before {
				t.Fatal("拒绝后快照变化")
			}
		})
	}
}

func TestExternalAllowsAtomicRegularFileReplacement(t *testing.T) {
	s := NewStore("", "")
	doc := loadExternalTestDocument(t, s, "原文")
	replacement := filepath.Join(filepath.Dir(doc.Path), "replacement.md")
	writeExternalTestDocument(t, replacement, "替换内容")
	if err := os.Rename(replacement, doc.Path); err != nil {
		t.Fatal(err)
	}
	state, err := s.CheckExternal(doc.ID)
	if err != nil || !state.Changed || state.Missing {
		t.Fatalf("应允许普通文件原子替换：%+v %v", state, err)
	}
	reloaded, err := s.Reload(doc.ID, false)
	if err != nil || reloaded.ID != doc.ID || reloaded.Content != "替换内容" || reloaded.Dirty {
		t.Fatalf("原子替换重新加载失败：%+v %v", reloaded, err)
	}
	if s.Current() != reloaded {
		t.Fatal("当前标签未更新")
	}
	state, err = s.CheckExternal(doc.ID)
	if err != nil || state != (ExternalState{}) {
		t.Fatalf("当前标签摘要未更新：%+v %v", state, err)
	}
}

func TestExternalOriginalSymlinkUsesResolvedPath(t *testing.T) {
	s := NewStore("", "")
	target := filepath.Join(t.TempDir(), "target.md")
	link := filepath.Join(t.TempDir(), "link.md")
	writeExternalTestDocument(t, target, "原文")
	if err := os.Symlink(target, link); err != nil {
		t.Skip(err)
	}
	doc, err := s.Load(link)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	writeExternalTestDocument(t, doc.Path, "外部修改")
	state, err := s.CheckExternal(doc.ID)
	if err != nil || !state.Changed || state.Missing {
		t.Fatalf("应检查已解析实际路径：%+v %v", state, err)
	}
	if loaded, err := s.Reload(doc.ID, false); err != nil || loaded.Content != "外部修改" || loaded.ID != doc.ID {
		t.Fatalf("应加载实际路径：%+v %v", loaded, err)
	}
}

func TestConcurrentCheckExternalWithGetDraftList(t *testing.T) {
	s := NewStore("未命名.md", "草稿初值")
	untitled := s.Current()
	unchanged := loadExternalTestDocument(t, s, "稳定内容\n")
	modified := loadExternalTestDocument(t, s, "原始磁盘内容\n")
	writeExternalTestDocument(t, modified.Path, "外部已修改内容\n")

	const workers = 6
	const iters = 40
	var wg sync.WaitGroup
	errs := make(chan error, workers*iters)

	// 并发检查外部变化：分别校验未变更、已变更和未命名文档的状态。
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				st, err := s.CheckExternal(unchanged.ID)
				if err != nil || st.Changed || st.Missing {
					errs <- fmt.Errorf("未修改文档检查异常：%+v %v", st, err)
					return
				}
				st, err = s.CheckExternal(modified.ID)
				if err != nil || !st.Changed || st.Missing {
					errs <- fmt.Errorf("已修改文档检查异常：%+v %v", st, err)
					return
				}
				st, err = s.CheckExternal(untitled.ID)
				if err != nil || st.Changed || st.Missing {
					errs <- fmt.Errorf("未命名文档检查异常：%+v %v", st, err)
					return
				}
			}
		}()
	}

	// 并发调用 Get、Draft、List，验证锁外磁盘 I/O 下无数据竞态且状态一致。
	for w := 0; w < 3; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				if err := s.Draft(unchanged.ID, fmt.Sprintf("并发草稿-%d-%d", id, i)); err != nil {
					errs <- fmt.Errorf("Draft 失败：%v", err)
					return
				}
				if got, ok := s.Get(unchanged.ID); !ok || got.ID != unchanged.ID || !got.Dirty {
					errs <- fmt.Errorf("Get 未命中或状态异常：ok=%v doc=%+v", ok, got)
					return
				}
				if got, ok := s.Get(modified.ID); !ok || got.ID != modified.ID {
					errs <- fmt.Errorf("Get 当前文档失败：ok=%v doc=%+v", ok, got)
					return
				}
				list := s.List()
				if len(list) != 3 {
					errs <- fmt.Errorf("List 文档数量异常：got %d, want 3", len(list))
					return
				}
			}
		}(w)
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

func TestReloadBOMCRLFAndErrorPaths(t *testing.T) {
	s := NewStore("未命名.md", "未保存")
	untitled := s.Current()
	doc := loadExternalTestDocument(t, s, "第一行\n第二行\n")

	// 未保存文档与不存在 ID 的错误路径。
	if _, err := s.Reload(untitled.ID, false); err == nil {
		t.Fatal("未保存文档不应允许 Reload")
	}
	if _, err := s.Reload("missing-id", true); !errors.Is(err, ErrStale) {
		t.Fatalf("不存在 ID 应返回 ErrStale，实际 %v", err)
	}

	// 脏文档在 discardDirty=false 时必须拒绝且保留草稿。
	if err := s.Draft(doc.ID, "本地未保存编辑"); err != nil {
		t.Fatal(err)
	}
	writeExternalTestDocument(t, doc.Path, "\ufeff外部第一行\r\n外部第二行\r\n")
	if _, err := s.Reload(doc.ID, false); !errors.Is(err, ErrDirty) {
		t.Fatalf("脏文档未确认丢弃应返回 ErrDirty，实际 %v", err)
	}
	if cur, ok := s.Get(doc.ID); !ok || cur.Content != "本地未保存编辑" || !cur.Dirty {
		t.Fatalf("拒绝重载后草稿状态异常：ok=%v doc=%+v", ok, cur)
	}

	// 确认丢弃后重载应规范化内存内容并记录 BOM+CRLF，再保存时完整保留。
	reloaded, err := s.Reload(doc.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Content != "外部第一行\n外部第二行\n" || reloaded.Dirty {
		t.Fatalf("重载后正文或脏标记异常：%+v", reloaded)
	}
	if err := s.Draft(doc.ID, "保存第一行\n保存第二行\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(doc.ID, "保存第一行\n保存第二行\n", doc.Path); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(doc.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "\ufeff保存第一行\r\n保存第二行\r\n" {
		t.Fatalf("重载后保存未保留 BOM+CRLF：%q", raw)
	}

	// 并发执行 Reload 与 Get/List/CheckExternal 无竞态。
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				if _, err := s.Reload(doc.ID, false); err != nil {
					errs <- err
					return
				}
				if got, ok := s.Get(doc.ID); !ok || got.Dirty {
					errs <- fmt.Errorf("并发 Reload 后 Get 异常：ok=%v doc=%+v", ok, got)
					return
				}
				if st, err := s.CheckExternal(doc.ID); err != nil || st.Changed || st.Missing {
					errs <- fmt.Errorf("并发 Reload 后 CheckExternal 异常：%+v %v", st, err)
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}
