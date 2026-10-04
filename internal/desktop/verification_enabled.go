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
	if mode := siteShotMode(); mode != "" {
		if verificationPath, err = prepareSiteShot(root, mode); err != nil {
			panic(err)
		}
	}
	if _, err := files.store.Load(verificationPath); err != nil {
		panic(err)
	}
	mygo.SetFrontend(os.DirFS(filepath.Join(root, ".verification-web")))
	mygo.App.SetName("Leafmark 验证")
}

func startVerification(win *mygo.Window, files *Files) {
	var once sync.Once
	win.Page().OnDidFinishLoad(func() { once.Do(func() { go verifyNative(win, files) }) })
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
	eval := func(code string) (any, error) { return win.Page().EvalContext(ctx, code) }
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
	if mode := siteShotMode(); mode != "" {
		if err := captureSiteShots(win, eval, mode); err != nil {
			fmt.Fprintln(os.Stderr, "官网截图失败：", err)
			mygo.App.Exit(1)
			return
		}
		mygo.App.Exit(0)
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
	result, err = eval(`const test=window.leafmarkVerification; let notify, complete, calls=0, restarts=0, unsubscribed=0; const status={version:'0.3.3',enabled:true,available:'0.3.4',notes:'# 问题修复\n\n- 打开文件更平稳。',installed:false}; const dialog=test.openUpdateDialog(status,()=>{calls++;return new Promise(resolve=>complete=resolve);},{onProgress:listener=>{notify=listener;return ()=>unsubscribed++;},restart:async()=>{restarts++;}}); const confirm=dialog.querySelector('.primary'), cancel=dialog.querySelector('.dialog-actions button'), progress=dialog.querySelector('progress'); if(!progress.hidden) throw Error('下载开始前进度条应隐藏'); confirm.click(); if(!confirm.disabled || !cancel.disabled || progress.hidden || progress.hasAttribute('value')) throw Error('下载开始状态错误'); confirm.click(); if(calls!==1) throw Error('重复发起安装'); notify({downloaded:45,total:100}); if(progress.value!==45 || !dialog.textContent.includes('45%')) throw Error('真实下载进度未显示'); const original=document.documentElement.dataset.theme; document.documentElement.dataset.theme='dark'; window.updateVerification={dialog,confirm,cancel,progress,complete,notify,get restarts(){return restarts;},get unsubscribed(){return unsubscribed;},original}; return {realProgress:true,duplicateInstallPrevented:true};`)
	if err != nil {
		fail(err)
		return
	}
	results["updateDownload"] = result
	if png, err := win.CapturePage(); err == nil {
		os.WriteFile("verification/native-update-progress.png", png, 0644)
	} else {
		fail(err)
		return
	}
	result, err = eval(`const state=window.updateVerification; state.notify({downloaded:100,total:100}); if(state.confirm.textContent!=='确定升级' || !state.confirm.disabled || !state.dialog.textContent.includes('正在校验并安装')) throw Error('下载完成提前允许重启'); state.complete(); await new Promise(r=>setTimeout(r,0)); if(state.progress.value!==100 || state.confirm.textContent!=='重启应用' || state.confirm.disabled || state.cancel.textContent!=='稍后') throw Error('安装完成状态错误'); state.confirm.click(); await new Promise(r=>setTimeout(r,0)); if(state.restarts!==1) throw Error('未调用重启接口'); return {installationWait:true,restartButton:true,restartCalled:true};`)
	if err != nil {
		fail(err)
		return
	}
	results["updateInstalled"] = result
	if png, err := win.CapturePage(); err == nil {
		os.WriteFile("verification/native-update-installed.png", png, 0644)
	} else {
		fail(err)
		return
	}
	result, err = eval(`const state=window.updateVerification; state.dialog.close(); await new Promise(r=>setTimeout(r,0)); if(state.unsubscribed!==1) throw Error('关闭弹窗未解绑进度事件'); document.documentElement.dataset.theme=state.original; let attempts=0; const dialog=window.leafmarkVerification.openUpdateDialog({version:'0.3.3',enabled:true,available:'0.3.4',notes:'',installed:false},async()=>{if(++attempts===1) throw Error('模拟下载失败');},{onProgress:()=>()=>{},restart:async()=>{throw Error('模拟重启失败');}}); const confirm=dialog.querySelector('.primary'), progress=dialog.querySelector('progress'); confirm.click(); await new Promise(r=>setTimeout(r,0)); if(confirm.disabled || !progress.hidden || !dialog.textContent.includes('模拟下载失败')) throw Error('安装失败未恢复重试'); confirm.click(); await new Promise(r=>setTimeout(r,0)); if(attempts!==2 || confirm.textContent!=='重启应用') throw Error('安装重试失败'); confirm.click(); await new Promise(r=>setTimeout(r,0)); if(confirm.disabled || confirm.textContent!=='重启应用' || !dialog.textContent.includes('模拟重启失败')) throw Error('重启失败无法重试'); dialog.close(); await new Promise(r=>setTimeout(r,0)); delete window.updateVerification; return {unsubscribe:true,installRetry:true,restartRetry:true};`)
	if err != nil {
		fail(err)
		return
	}
	results["updateRetry"] = result
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
	win.Page().Reload()
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
	// 模拟 Finder 拖入或“打开方式”交给应用的文档：后端排队后通知页面，页面在空闲时打开为新标签。
	dropped := filepath.Join(filepath.Dir(verificationPath), "拖入验证.md")
	if err := os.WriteFile(dropped, []byte("# 拖入验证\n\n系统交给应用的文档 🌿\n"), 0644); err != nil {
		fail(err)
		return
	}
	tabs, err := eval(`return document.querySelectorAll('.file-tab').length;`)
	if err != nil {
		fail(err)
		return
	}
	_ = OpenRequested.Emit(win, files.requestOpen([]string{dropped, filepath.Join(filepath.Dir(dropped), "图片验证.png")}))
	if err := wait(fmt.Sprintf("window.leafmarkVerification.document().path.endsWith('拖入验证.md') && window.leafmarkVerification.editor.state.doc.toString().includes('系统交给应用的文档') && document.querySelectorAll('.file-tab').length===%v+1", tabs)); err != nil {
		fail(err)
		return
	}
	results["openRequested"] = true
	fmt.Println("通过：系统交给应用的 Markdown 文档打开为新标签，忽略非 Markdown 文件")
	result, err = eval(`document.querySelector('#documents-tab').click(); await new Promise(r=>setTimeout(r,100)); const button=[...document.querySelectorAll('#documents button')].find(button=>button.textContent.includes('工作区验证.md')); if(!button) throw Error('工作区文件树未显示 '+document.querySelector('#documents').innerText); window.leafmarkVerification.editor.focus(); const down=new MouseEvent('mousedown',{bubbles:true,cancelable:true,button:0}); button.dispatchEvent(down); if(!down.defaultPrevented) throw Error('文件导航未保留首次点击'); button.click(); await new Promise(r=>setTimeout(r,150)); if(!window.leafmarkVerification.document().path.endsWith('工作区验证.md')) throw Error('工作区文件未加载'); document.querySelector('#reading-toggle').click(); await new Promise(r=>setTimeout(r,200)); const image=document.querySelector('#reading-view img'); if(!image?.src.startsWith('data:image/png')) throw Error('本地相对图片未解析'); return {folderTree:true,workspaceOpen:true,relativeImage:true};`)
	if err != nil {
		fail(err)
		return
	}
	results["workspace"] = result
	result, err = eval(`const test=window.leafmarkVerification; document.querySelector('#reading-toggle').click(); const button=[...document.querySelectorAll('.fb-tree .fb-node-open')].find(button=>button.title==='工作区验证.md'); const nodes=[...document.querySelectorAll('.fb-tree button')]; const state=test.editor.state; const before=getComputedStyle(button).opacity; const samples=[]; const observer=new MutationObserver(()=>{samples.push(getComputedStyle(button).opacity);}); observer.observe(document.querySelector('.fb-browser'),{subtree:true,attributes:true,attributeFilter:['disabled','aria-busy']}); button.click(); await new Promise(r=>setTimeout(r,250)); observer.disconnect(); if(!nodes.every(node=>node.isConnected)) throw Error('打开文档重建了未变化的文件树'); if(samples.some(value=>value!==before)) throw Error('打开文档时文件树亮度变化：'+samples); if(test.editor.state.doc!==state.doc) throw Error('重新打开已打开文档丢失了正文状态'); document.querySelector('#reading-toggle').click(); const reading=document.querySelector('#reading-view'); const heading=reading.querySelector('h1'); button.click(); await new Promise(r=>setTimeout(r,250)); if(!heading?.isConnected) throw Error('重新打开已打开文档重复渲染阅读正文'); return {stableTree:true,stableBrightness:true,preservedDocument:true,stableReading:true};`)
	if err != nil {
		fail(err)
		return
	}
	results["documentSwitchStability"] = result
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
	result, err = eval(`const test=window.leafmarkVerification; const view=test.editor; const original=view.state.doc.toString(); const source='| A | B |\n| --- | ---: |\n| x | y |\n| z | w |'; view.dispatch({changes:{from:view.state.doc.length,insert:'\n\n'+source},selection:{anchor:0}}); document.querySelector('#documents-tab').focus(); await new Promise(r=>setTimeout(r,80)); const preview=()=>document.querySelector('.lm-table-preview'); const open=()=>preview().querySelector('.lm-table-actions').click(); const choose=label=>[...preview().querySelectorAll('[role=menuitem]')].find(item=>item.textContent===label).click(); preview().scrollIntoView({block:'center'}); const button=preview().querySelector('.lm-table-actions'); button.focus(); const br=button.getBoundingClientRect(), tr=preview().querySelector('table').getBoundingClientRect(); if(br.right>tr.left || Math.abs(br.top-tr.top)>1) throw Error('操作柄未位于表格左侧'); open(); preview().querySelector('[role=menuitem]').dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true})); if(preview().querySelector('[role=menu]')) throw Error('Escape 未关闭菜单'); preview().querySelectorAll('tbody tr')[1].dispatchEvent(new MouseEvent('mouseenter')); open(); choose('在上方插入行'); if(!view.state.doc.toString().endsWith('| x | y |\n|  |  |\n| z | w |')) throw Error('上方插行位置错误'); test.undo(); document.querySelector('#documents-tab').focus(); await new Promise(r=>setTimeout(r,30)); preview().querySelectorAll('tbody tr')[1].dispatchEvent(new MouseEvent('mouseenter')); open(); choose('在下方插入行'); if(!view.state.doc.toString().endsWith('| z | w |\n|  |  |')) throw Error('下方插行位置错误'); test.undo(); document.querySelector('#documents-tab').focus(); await new Promise(r=>setTimeout(r,30)); open(); choose('删除表格'); if(view.state.doc.toString()!==original+'\n\n') throw Error('删除影响了表格外正文'); if(!test.undo() || !view.state.doc.toString().endsWith(source)) throw Error('删除表格无法撤销'); document.querySelector('#documents-tab').focus(); await new Promise(r=>setTimeout(r,30)); preview().scrollIntoView({block:'center'}); preview().querySelector('.lm-table-actions').focus(); window.tableVerificationOriginal=original; return {leftHandle:true,escape:true,insertAbove:true,insertBelow:true,delete:true,undo:true};`)
	if err != nil {
		fail(err)
		return
	}
	results["tableActions"] = result
	if png, err := win.CapturePage(); err == nil {
		os.WriteFile("verification/native-table-actions.png", png, 0644)
	} else {
		fail(err)
		return
	}
	_, err = eval(`const view=window.leafmarkVerification.editor; view.dispatch({changes:{from:0,to:view.state.doc.length,insert:window.tableVerificationOriginal},selection:{anchor:0}}); return true;`)
	if err != nil {
		fail(err)
		return
	}
	result, err = eval(`const test=window.leafmarkVerification; const view=test.editor; const before=view.state.doc.toString(); const top=view.scrollDOM.scrollTop; const fence='\x60\x60\x60'; view.dispatch({changes:{from:0,insert:'| 列 |\n| --- |\n| **粗** [链](https://example.com) ![图](./图片验证.png) |\n\n'+fence+'js\nconst a = "s"; // c\nfunction f(){ return 1 }\n'+fence+'\n\n'},selection:{anchor:view.state.doc.length}}); view.scrollDOM.scrollTop=0; document.querySelector('#documents-tab').focus(); await new Promise(r=>setTimeout(r,200)); const cell=document.querySelector('.lm-table-preview td'); if(cell.querySelector('strong')?.textContent!=='粗' || cell.querySelector('a')?.getAttribute('href')!=='https://example.com' || cell.querySelector('.lm-image-fallback')?.textContent!=='图' || cell.querySelector('img')) throw Error('表格预览未渲染单元格行内格式：'+cell.innerHTML); const code=document.querySelector('.lm-code-preview code'); const colors=['keyword','string','number','comment','title'].map(name=>{const node=code.querySelector('.hljs-'+name); if(!node) throw Error('缺少高亮类别 '+name); return getComputedStyle(node).color;}); const [keyword,string,number,comment,title]=colors; if(new Set([keyword,string,number,title]).size!==1 || new Set([keyword,comment,getComputedStyle(code).color]).size!==3) throw Error('代码高亮配色不符：'+colors); view.dispatch({changes:{from:0,to:view.state.doc.length,insert:before},selection:{anchor:0}}); view.scrollDOM.scrollTop=top; document.querySelector('#documents-tab').focus(); await new Promise(r=>setTimeout(r,80)); if(view.state.doc.toString()!==before) throw Error('未还原文档'); return {tableInline:true,imageFallback:true,syntaxColors:colors.length};`)
	if err != nil {
		fail(err)
		return
	}
	results["previewRendering"] = result
	fmt.Println("通过：表格单元格行内格式与代码高亮")
	result, err = eval(`const view=window.leafmarkVerification.editor; window.roundedVerificationOriginal=view.state.doc.toString(); const fence='\x60\x60\x60'; const source='| A | B |\n| --- | --- |\n| x | y |\n\n'+fence+'js\nconst a = 1;\n'+fence; view.dispatch({changes:{from:view.state.doc.length,insert:'\n\n'+source},selection:{anchor:0}}); document.querySelector('#documents-tab').focus(); await new Promise(r=>setTimeout(r,120)); const radius=getComputedStyle(document.documentElement).getPropertyValue('--control-radius').trim(); const corners=['borderTopLeftRadius','borderTopRightRadius','borderBottomLeftRadius','borderBottomRightRadius']; const rounded=(name,node)=>{ if(!node) throw Error('未渲染'+name); const style=getComputedStyle(node); for(const corner of corners) if(style[corner]!==radius) throw Error(name+'圆角不一致：'+style[corner]); if(style.overflowX==='visible') throw Error(name+'未裁切内容'); }; const scroll=document.querySelector('.lm-table-preview .lm-table-scroll'); const blocks=document.querySelectorAll('.lm-block-preview'); const block=blocks[blocks.length-1]; rounded('原位表格',scroll); rounded('原位代码块',block); const cell=getComputedStyle(scroll.querySelector('th')); if(cell.borderTopWidth!=='0px' || cell.borderLeftWidth!=='0px') throw Error('表格单元格仍绘制外侧边框'); block.scrollIntoView({block:'end'}); await new Promise(r=>setTimeout(r,80)); return {radius};`)
	if err != nil {
		fail(err)
		return
	}
	results["roundedBlocks"] = result
	if png, err := win.CapturePage(); err == nil {
		os.WriteFile("verification/native-rounded-blocks.png", png, 0644)
	} else {
		fail(err)
		return
	}
	_, err = eval(`const view=window.leafmarkVerification.editor; document.querySelector('#reading-toggle').click(); await new Promise(r=>setTimeout(r,400)); const radius=getComputedStyle(document.documentElement).getPropertyValue('--control-radius').trim(); const reading=document.querySelector('#reading-view'); const last=selector=>{ const nodes=reading.querySelectorAll(selector); return nodes[nodes.length-1]; }; for(const [name,node] of [['阅读表格',last('table')],['阅读代码块',last('pre')]]) { if(!node) throw Error('未渲染'+name); const style=getComputedStyle(node); if(style.borderTopLeftRadius!==radius || style.borderBottomRightRadius!==radius) throw Error(name+'圆角不一致：'+style.borderTopLeftRadius); } document.querySelector('#reading-toggle').click(); await new Promise(r=>setTimeout(r,120)); view.dispatch({changes:{from:0,to:view.state.doc.length,insert:window.roundedVerificationOriginal},selection:{anchor:0}}); return true;`)
	if err != nil {
		fail(err)
		return
	}
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
