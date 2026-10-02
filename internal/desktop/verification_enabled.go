//go:build verification

package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"leafmark/internal/workspace"
)

var verificationPath string

// 验证入口仅在显式开启 verification 构建标签时编译。
func prepareVerification(files *Files) {
	root, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	backend, err := workspace.NewAt(filepath.Join(root, ".verification-data"))
	if err != nil {
		panic(err)
	}
	files.workspace.once.Do(func() { files.workspace.store = backend })
	if _, err := backend.SelectFolder(filepath.Join(root, "verification")); err != nil {
		panic(err)
	}
	imageRaw, err := os.ReadFile(filepath.Join(root, "resources", "icon.png"))
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(root, "verification", "图片验证.png"), imageRaw, 0600); err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(root, "verification", "工作区验证.md"), []byte("# 工作区验证\n\n![本地图片](./图片验证.png)\n"), 0600); err != nil {
		panic(err)
	}
	verificationPath = filepath.Join(root, "verification", "本机读写验证.md")
	raw, err := os.ReadFile(filepath.Join(root, "verification", "输入验证.md"))
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(verificationPath, raw, 0644); err != nil {
		panic(err)
	}
	if _, err := files.store.Load(verificationPath); err != nil {
		panic(err)
	}
	mygo.SetFrontend(os.DirFS(filepath.Join(root, ".verification-web")))
	mygo.App.SetName("Leafmark 验证")
}

func startVerification(win *mygo.Window, files *Files) {
	var once sync.Once
	win.OnDidFinishLoad(func() { once.Do(func() { go verifyNative(win, files) }) })
	// 超时退出，避免自动验证留下无人处理的窗口或无限等待。
	go func() {
		time.Sleep(45 * time.Second)
		fmt.Fprintln(os.Stderr, "原生验证超时")
		mygo.App.Exit(1)
	}()
}

func verifyNative(win *mygo.Window, files *Files) {
	results := map[string]any{"platform": "macOS WKWebView", "passed": false}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	eval := func(code string) (any, error) { return win.EvalContext(ctx, code) }
	fail := func(err error) {
		results["error"] = err.Error()
		raw, _ := json.MarshalIndent(results, "", "  ")
		os.WriteFile("verification/native-results.json", raw, 0644)
		fmt.Fprintln(os.Stderr, "原生验证失败：", err)
		mygo.App.Exit(1)
	}
	wait := func(expression string) error {
		for attempts := 0; attempts < 200; attempts++ {
			result, err := eval(expression)
			if err == nil && result == true {
				return nil
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			time.Sleep(50 * time.Millisecond)
		}
		return fmt.Errorf("未满足验证条件：%s", expression)
	}
	if err := wait("Boolean(window.leafmarkVerification && window.leafmarkVerification.document().path)"); err != nil {
		fail(err)
		return
	}
	result, err := eval(`const heading = document.querySelector('.md-h1')?.textContent; const bold = document.querySelector('.md-strong')?.textContent; const tasks = document.querySelectorAll('.task-box').length; if (!heading?.includes('最小功能验证') || bold !== '加粗文字' || tasks !== 2) throw Error('初始 Markdown 排版不匹配'); return {heading,bold,tasks,native:window.mygo.platform};`)
	if err != nil {
		fail(err)
		return
	}
	results["initialRendering"] = result
	result, err = eval(`const test=window.leafmarkVerification; const view=test.editor; view.focus(); const before={text:view.state.doc.toString(),anchor:view.state.selection.main.anchor,head:view.state.selection.main.head,scroll:view.scrollDOM.scrollTop,tabs:document.querySelectorAll('.file-tab').length}; window.settingsVerificationBefore=before; document.querySelector('#settings-toggle').click(); const page=document.querySelector('.leafmark-settings'); const rect=page.getBoundingClientRect(); if(page.tagName==='DIALOG' || document.querySelector('dialog[open]') || rect.x!==0 || rect.y!==0 || rect.width!==innerWidth || rect.height!==innerHeight) throw Error('设置未替换整个窗口'); if(!document.querySelector('#app').inert || getComputedStyle(document.querySelector('#app')).visibility!=='hidden') throw Error('后台编辑页面仍可交互'); if(!page.contains(document.activeElement)) throw Error('设置页未获得焦点'); document.querySelector('#settings-toggle').click(); if(document.querySelectorAll('.leafmark-settings').length!==1) throw Error('重复打开设置页'); return {fullWindow:true,backgroundInactive:true,singlePage:true};`)
	if err != nil {
		fail(err)
		return
	}
	results["settingsPage"] = result
	result, err = eval(`const original=document.documentElement.dataset.theme; for(const theme of ['light','dark']) { document.documentElement.dataset.theme=theme; const main=document.querySelector('.sidebar'); const settings=document.querySelector('.settings-sidebar'); const a=getComputedStyle(main), b=getComputedStyle(settings); const ar=main.getBoundingClientRect(), br=settings.getBoundingClientRect(); if(ar.width!==br.width || ar.x!==br.x || ar.y!==br.y || ar.height!==br.height) throw Error('设置与文档侧栏尺寸不一致'); for(const property of ['backgroundImage','boxShadow','borderRadius','backdropFilter']) if(a[property]!==b[property]) throw Error(theme+' 侧栏材质不一致：'+property); if(getComputedStyle(main,'::before').backgroundImage!==getComputedStyle(settings,'::before').backgroundImage) throw Error('玻璃描边不一致'); } document.documentElement.dataset.theme=original; return {light:true,dark:true,width:true,glass:true};`)
	if err != nil {
		fail(err)
		return
	}
	results["settingsSidebar"] = result
	if png, err := win.CapturePage(); err == nil {
		os.WriteFile("verification/native-settings.png", png, 0644)
	} else {
		fail(err)
		return
	}
	result, err = eval(`document.querySelector('.settings-tab').dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true})); const view=window.leafmarkVerification.editor; const before=window.settingsVerificationBefore; if(document.querySelector('.leafmark-settings') || document.querySelector('#app').inert || getComputedStyle(document.querySelector('#app')).visibility==='hidden') throw Error('未恢复书写页面'); if(view.state.doc.toString()!==before.text || view.state.selection.main.anchor!==before.anchor || view.state.selection.main.head!==before.head || view.scrollDOM.scrollTop!==before.scroll || document.querySelectorAll('.file-tab').length!==before.tabs) throw Error('设置返回改变了编辑状态'); if(document.activeElement!==view.contentDOM) throw Error('未恢复编辑器焦点'); return {escape:true,document:true,selection:true,scroll:true,tabs:true,focus:true};`)
	if err != nil {
		fail(err)
		return
	}
	results["settingsReturn"] = result
	result, err = eval(`document.querySelector('#settings-toggle').click(); [...document.querySelectorAll('.settings-tab')].find(tab=>tab.textContent.includes('通用')).click(); await new Promise(r=>setTimeout(r,150)); const panel=document.querySelector('.settings-panel:not([hidden])'); if(!panel.textContent.includes('软件更新') || !panel.textContent.includes('当前版本：') || !panel.textContent.includes('开发版或安装目录不可写')) throw Error('软件更新状态不匹配'); const check=[...panel.querySelectorAll('button')].find(button=>button.textContent==='检查更新'); if(!check?.disabled) throw Error('验证构建不应允许安装更新'); document.querySelector('.settings-close').click(); return {entry:true,developmentDisabled:true};`)
	if err != nil {
		fail(err)
		return
	}
	results["softwareUpdates"] = result
	result, err = eval(`const tab=document.querySelector('.file-tab.active'); const sidebar=document.querySelector('.sidebar'); const rect=tab.getBoundingClientRect(); const radius=getComputedStyle(tab).borderTopLeftRadius; if(rect.top!==sidebar.getBoundingClientRect().top || rect.height!==36 || radius!=='10px') throw Error('标签顶部对齐、尺寸或方形圆角不匹配'); return {top:rect.top,height:rect.height,radius};`)
	if err != nil {
		fail(err)
		return
	}
	results["tabAppearance"] = result
	fmt.Println("通过：原生窗口、UTF-8 文档加载、Markdown 排版和标签圆角")

	result, err = eval(`const test = window.leafmarkVerification; const view = test.editor; const original = view.state.doc.toString(); view.dispatch({changes:{from:view.state.doc.length,insert:'\n中文输入验证 🌿\n'}}); await test.flush(); const inserted = view.state.doc.toString(); if (!inserted.includes('中文输入验证 🌿')) throw Error('中文插入失败'); if (!test.undo() || view.state.doc.toString() !== original) throw Error('撤销失败'); await test.flush(); if (!test.redo() || view.state.doc.toString() !== inserted) throw Error('重做失败'); await test.flush(); return {unicode:true,undo:true,redo:true,dirty:test.document().dirty};`)
	if err != nil {
		fail(err)
		return
	}
	results["editing"] = result
	fmt.Println("通过：中文文本插入、撤销、重做和草稿 IPC")
	mygo.App.Focus()
	win.Focus()
	time.Sleep(150 * time.Millisecond)

	result, err = eval(`const test = window.leafmarkVerification; const view = test.editor; const before = view.state.doc.toString(); document.querySelector('#mode-toggle').click(); if (!document.querySelector('.cm-content').textContent.includes('# 最小功能验证')) throw Error('源码模式失败'); document.querySelector('#mode-toggle').click(); if (view.state.doc.toString() !== before) throw Error('模式切换改变了原文'); const from = before.indexOf('中文输入验证'); view.dispatch({selection:{anchor:from,head:from+6},scrollIntoView:true}); view.focus(); await new Promise(r=>setTimeout(r,80)); const toolbar=document.querySelector('#format-toolbar'); if (toolbar.hidden && view.hasFocus) throw Error('格式工具栏未出现'); document.querySelector('[data-format="bold"]').click(); await test.flush(); if (!view.state.doc.toString().includes('**中文输入验证**')) throw Error('加粗操作失败'); return {sourceMode:true,toolbar:!toolbar.hidden,toolbarNote:view.hasFocus?'已验证':'后台 WebView 未获焦点；工具栏交互由浏览器验证',bold:true};`)
	if err != nil {
		fail(err)
		return
	}
	results["formatting"] = result
	fmt.Println("通过：源码切换、选区工具栏和加粗")

	expected := files.store.Current().Content
	result, err = eval(`document.querySelector('#save-file').click(); return true;`)
	if err != nil {
		fail(err)
		return
	}
	if err := wait("!window.leafmarkVerification.document().dirty && document.querySelector('#save-status').textContent === '已保存'"); err != nil {
		fail(err)
		return
	}
	raw, err := os.ReadFile(verificationPath)
	if err != nil || string(raw) != expected {
		fail(fmt.Errorf("保存内容不匹配：%v", err))
		return
	}
	results["diskSave"] = true
	fmt.Println("通过：保存按钮、Go IPC 和真实磁盘写入")
	if _, err := files.store.Load(verificationPath); err != nil {
		fail(err)
		return
	}
	win.Reload()
	time.Sleep(200 * time.Millisecond)
	if err := wait("Boolean(window.leafmarkVerification && window.leafmarkVerification.document().path && window.leafmarkVerification.document().content.includes('**中文输入验证**'))"); err != nil {
		fail(err)
		return
	}
	results["diskReload"] = true
	result, err = eval(`document.querySelector('#theme-toggle').click(); document.querySelector('#hide-sidebar').click(); const hidden=document.querySelector('#app').classList.contains('sidebar-hidden'); document.querySelector('#show-sidebar').click(); if(!hidden || document.querySelector('#app').classList.contains('sidebar-hidden')) throw Error('侧栏切换失败'); return {theme:document.documentElement.dataset.theme,sidebar:true};`)
	if err != nil {
		fail(err)
		return
	}
	results["appearance"] = result
	result, err = eval(`const test=window.leafmarkVerification; const firstID=test.document().id; const original=test.editor.state.doc.toString(); document.querySelector('#new-file').click(); await new Promise(r=>setTimeout(r,100)); if(test.document().id===firstID) throw Error('新标签未创建'); const secondID=test.document().id; test.editor.dispatch({changes:{from:0,insert:'# 第二标签\n\n独立草稿 🌿'}}); await test.flush(); [...document.querySelectorAll('.file-tab')].find(tab=>tab.textContent.includes('本机读写验证.md')).querySelector('button').click(); await new Promise(r=>setTimeout(r,100)); if(test.document().id!==firstID || test.editor.state.doc.toString()!==original) throw Error('第一标签内容丢失'); [...document.querySelectorAll('.file-tab')].at(-1).querySelector('button').click(); await new Promise(r=>setTimeout(r,100)); if(test.document().id!==secondID || !test.editor.state.doc.toString().includes('独立草稿')) throw Error('第二标签草稿丢失'); [...document.querySelectorAll('.file-tab')].at(-1).querySelector('.close-tab').click(); await new Promise(r=>setTimeout(r,80)); if(!document.querySelector('#unsaved-dialog').open) throw Error('关闭草稿未提示'); document.querySelector('#unsaved-dialog button[value=cancel]').click(); await new Promise(r=>setTimeout(r,80)); if(test.document().id!==secondID) throw Error('取消关闭丢失标签'); return {tabs:true,independentDrafts:true,cancelClose:true};`)
	if err != nil {
		fail(err)
		return
	}
	results["tabs"] = result
	fmt.Println("通过：原生多标签、独立草稿和取消关闭")
	result, err = eval(`[...document.querySelectorAll('.file-tab')].at(-1).querySelector('.close-tab').click(); await new Promise(r=>setTimeout(r,80)); document.querySelector('#unsaved-dialog button[value=discard]').click(); await new Promise(r=>setTimeout(r,100)); document.querySelector('#settings-toggle').click(); const switches=document.querySelectorAll('.leafmark-settings [role=switch]'); const toggle=[...switches].find(button=>button.getAttribute('aria-label')==='自动保存'); if(toggle) toggle.click(); else { const labels=[...document.querySelectorAll('.settings-row')]; const row=labels.find(row=>row.textContent.includes('自动保存')); if(!row) throw Error('自动保存设置未找到'); const toggle=row.querySelector('[role=switch]'); if(!toggle.checked) toggle.click(); } document.querySelector('.settings-close').click(); return true;`)
	if err != nil {
		fail(err)
		return
	} else {
		result, err = eval(`const test=window.leafmarkVerification; test.editor.dispatch({changes:{from:test.editor.state.doc.length,insert:'\n自动保存验证\n'}}); await test.flush(); return true;`)
		if err != nil {
			fail(err)
			return
		}
		if err := wait("!window.leafmarkVerification.document().dirty"); err != nil {
			fail(err)
			return
		}
		autoRaw, err := os.ReadFile(verificationPath)
		if err != nil || !strings.Contains(string(autoRaw), "自动保存验证") {
			fail(fmt.Errorf("自动保存磁盘内容错误：%v", err))
			return
		}
		results["autoSave"] = true
		fmt.Println("通过：两秒自动保存与磁盘内容")
	}
	result, err = eval(`document.querySelector('#documents-tab').click(); await new Promise(r=>setTimeout(r,100)); const button=[...document.querySelectorAll('#documents button')].find(button=>button.textContent.includes('工作区验证.md')); if(!button) throw Error('工作区文件树未显示 '+document.querySelector('#documents').innerText); window.leafmarkVerification.editor.focus(); const down=new MouseEvent('mousedown',{bubbles:true,cancelable:true,button:0}); button.dispatchEvent(down); if(!down.defaultPrevented) throw Error('文件导航未保留首次点击'); button.click(); await new Promise(r=>setTimeout(r,150)); if(!window.leafmarkVerification.document().path.endsWith('工作区验证.md')) throw Error('工作区文件未加载'); document.querySelector('#reading-toggle').click(); await new Promise(r=>setTimeout(r,200)); const image=document.querySelector('#reading-view img'); if(!image?.src.startsWith('data:image/png')) throw Error('本地相对图片未解析'); return {folderTree:true,workspaceOpen:true,relativeImage:true};`)
	if err != nil {
		fail(err)
		return
	}
	results["workspace"] = result
	result, err = eval(`const browser=document.querySelector('.fb-browser'); const search=browser.querySelector('.fb-search-box'); const toolbar=browser.querySelector('.fb-toolbar'); if(browser.querySelector('.fb-create-tools')) throw Error('常驻新建按钮仍存在'); if(!search.querySelector('svg') || search.nextElementSibling!==toolbar) throw Error('搜索框与目录行未遵循原型'); const input=search.querySelector('input'); input.focus(); const style=getComputedStyle(input); if(style.borderTopWidth!=='0px' || style.outlineStyle!=='none' || search.getBoundingClientRect().height>31) throw Error('搜索框样式不匹配'); input.blur(); toolbar.querySelector('.fb-more').click(); const menu=document.querySelector('.fb-menu'); if(!menu?.textContent.includes('在此新建文件…') || !menu.textContent.includes('在此新建文件夹…')) throw Error('目录创建菜单缺失'); document.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true})); return {prototypeLayout:true,compactSearch:true,creationMenu:true};`)
	if err != nil {
		fail(err)
		return
	}
	results["fileBrowserDesign"] = result
	result, err = eval(`const browser=document.querySelector('.fb-browser'); const toggle=browser.querySelector('.fb-root-toggle'); const tree=browser.querySelector('.fb-tree'); const name=browser.querySelector('.fb-workspace-choose'); if(!toggle || !name || toggle.contains(name) || !toggle.title.includes('折叠')) throw Error('根目录折叠入口未独立'); toggle.click(); if(!tree.hidden || toggle.getAttribute('aria-expanded')!=='false' || document.querySelector('dialog[open]')) throw Error('根目录折叠失败'); toggle.click(); if(tree.hidden || toggle.getAttribute('aria-expanded')!=='true') throw Error('根目录展开失败'); return {collapse:true,expand:true,separateWorkspaceSwitch:true};`)
	if err != nil {
		fail(err)
		return
	}
	results["workspaceCollapse"] = result
	fmt.Println("通过：原型文件导航布局、搜索框与目录创建菜单")
	fmt.Println("通过：原生工作区文件树、授权加载和本地相对图片")
	externalPath := files.store.Current().Path
	if err := os.WriteFile(externalPath, []byte("# 磁盘更新\n\n外部内容 🌿\n"), 0600); err != nil {
		fail(err)
		return
	}
	result, err = eval(`const test=window.leafmarkVerification; document.querySelector('#file-menu-toggle').click(); [...document.querySelectorAll('.action-menu button')].find(button=>button.textContent==='检查磁盘修改').click(); await new Promise(r=>setTimeout(r,100)); if(!document.querySelector('#save-status').textContent.includes('磁盘文件已修改')) throw Error('未检测到外部修改'); document.querySelector('#file-menu-toggle').click(); [...document.querySelectorAll('.action-menu button')].find(button=>button.textContent==='从磁盘重新加载…').click(); await new Promise(r=>setTimeout(r,150)); if(!test.editor.state.doc.toString().includes('外部内容 🌿') || test.document().dirty) throw Error('外部重新加载失败'); return {externalChange:true,reload:true};`)
	if err != nil {
		fail(err)
		return
	}
	results["external"] = result
	fmt.Println("通过：外部修改检测与磁盘重新加载")
	result, err = eval(`if(document.documentElement.dataset.theme==='dark') document.querySelector('#theme-toggle').click(); const test=window.leafmarkVerification; const view=test.editor; const original=view.state.doc.toString(); const sample='# 丰富语法验证\n\n[[文件|别名]] · [[文件#标题|别名]] · [[#标题]] · [[#^block-sample]]\n\n==高亮文本==\n\n![[不存在的音频.mp3]]\n\n> [!note]\n> 普通说明。\n\n> [!tip] 写作技巧\n> 一个实用的快捷方式。\n\n> [!warning] 提示标题\n> 提示正文\n\n## 标题\n\n段落内容。 ^block-sample\n'; view.dispatch({changes:{from:0,to:view.state.doc.length,insert:sample},selection:{anchor:0}}); view.contentDOM.blur(); document.querySelector('#more-actions').focus(); if(!document.querySelector('#reading-view').hidden) document.querySelector('#reading-toggle').click(); await new Promise(r=>setTimeout(r,200)); const wiki=document.querySelectorAll('#editor .lm-wiki-link').length; const highlight=document.querySelector('#editor .lm-highlight')?.textContent; const callout=Boolean(document.querySelector('#editor .lm-callout-warning')); const icons=document.querySelectorAll('#editor .lm-callout-title svg').length; const line=document.querySelector('#editor .lm-callout-warning'); const style=getComputedStyle(line); if(icons!==3 || style.borderLeftWidth!=='0px' || style.fontSize!=='13px' || style.backgroundColor!=='rgb(241, 235, 227)') throw Error('Callout 版式不匹配 '+JSON.stringify({icons,border:style.borderLeftWidth,font:style.fontSize,background:style.backgroundColor})); if(view.state.doc.toString()!==sample) throw Error('Callout 渲染修改了源码'); const embed=document.querySelector('#editor .lm-embed-placeholder')?.textContent; if(wiki!==4 || highlight!=='高亮文本' || !callout || !embed?.includes('暂未解析')) throw Error('丰富语法原位渲染不匹配 '+JSON.stringify({wiki,highlight,callout,embed})); return {wiki,highlight,callout,embed};`)
	if err != nil {
		fail(err)
		return
	}
	results["richSyntax"] = result
	fmt.Println("通过：原生双链、高亮、Callout 和嵌入占位渲染")
	result, err = eval(`const view=window.leafmarkVerification.editor; document.querySelector('#outline-tab').click(); const headings=[...document.querySelectorAll('#outline button')]; if(headings.length<2) throw Error('大纲标题不足'); view.focus(); const heading=headings[1]; const down=new MouseEvent('mousedown',{bubbles:true,cancelable:true,button:0}); heading.dispatchEvent(down); if(!down.defaultPrevented) throw Error('导航鼠标按下未保留编辑器焦点'); heading.click(); await new Promise(r=>setTimeout(r,80)); if(view.state.selection.main.head!==Number(heading.dataset.position)) throw Error('大纲首次点击未跳转'); if(!heading.isConnected) throw Error('大纲跳转重建了按钮'); document.querySelector('#documents-tab').click(); const browser=document.querySelector('.fb-browser'); const folder=browser.querySelector('button[aria-expanded]'); if(folder) { folder.click(); if(browser.querySelector('.fb-target,.fb-root-target')) throw Error('文件夹展开仍显示辅助区域'); } if(document.querySelector('.sidebar-bottom,#file-location')) throw Error('本地文件区域仍存在'); for(const selector of ['.file-tab','.sidebar-switch button','.fb-search-box']) { const node=document.querySelector(selector); if(!node || getComputedStyle(node).borderRadius!=='10px') throw Error('控件圆角不一致 '+selector); } return {singleClickOutline:true,stableOutline:true,cleanSidebar:true,controlRadius:'10px'};`)
	if err != nil {
		fail(err)
		return
	}
	results["sidebarNavigation"] = result
	fmt.Println("通过：导航首次点击、稳定大纲、侧栏布局与统一圆角")
	if png, err := win.CapturePage(); err == nil {
		os.WriteFile("verification/native-window.png", png, 0644)
		results["screenshot"] = "verification/native-window.png"
	} else {
		results["screenshotNote"] = err.Error()
	}
	if !strings.Contains(string(raw), "🌿") {
		fail(fmt.Errorf("emoji 保存丢失"))
		return
	}
	results["passed"] = true
	output, _ := json.MarshalIndent(results, "", "  ")
	os.WriteFile("verification/native-results.json", output, 0644)
	fmt.Println("通过：磁盘重新读取、主题和侧栏；原生验证全部完成")
	mygo.App.Exit(0)
}
