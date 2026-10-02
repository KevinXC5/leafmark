// 运行：ego-browser nodejs -e 'await import("/Users/kevinxc/Github/leafmark/scripts/verify-browser.mjs")'
// 每次独立验证仅创建一个 TaskSpace；首次探索可显式传入已有的本次验证空间。
const fs = await import('node:fs/promises');
const path = await import('node:path');
const root = path.resolve(path.dirname(new URL(import.meta.url).pathname), '..');
const output = path.join(root, 'verification');
await fs.mkdir(output, { recursive: true });
const os = await import('node:os');
// Vite 监听项目目录：运行中写入截图或 HTML 会触发页面重载，先在目录外暂存。
const staging = await fs.mkdtemp(path.join(os.tmpdir(), 'leafmark-e2e-'));
const task = await taskSpace(globalThis.LEAFMARK_VERIFY_SPACE ?? `Leafmark 功能验证 ${new Date().toISOString()}`);
const page = task.page('p1');
const keys = ['leafmark-settings', 'leafmark-shortcuts-v1', 'leafmark-recovery-v1'];
const result = { startedAt: new Date().toISOString(), url: 'http://localhost:5173', spaceId: task.spaceId, checks: [], artifacts: [], runtimeErrors: [], cleanup: {} };
console.log(`验证空间：${task.spaceId}`);
let original;
let fatal;
const editorSelector = '#editor .cm-content[contenteditable="true"]';
const assert = (condition, message) => { if (!condition) throw Error(message); };
async function state() {
  return page.evaluate(async () => {
    // 仅使用官方入口观察真实 state，不提交事务、不调用验证构建入口。
    const { EditorView } = await import('/node_modules/@codemirror/view/dist/index.js');
    const view = EditorView.findFromDOM(document.querySelector('#editor .cm-editor'));
    if (!view) throw Error('未找到 CodeMirror 编辑器');
    const selection = view.state.selection.main;
    return { text: view.state.doc.toString(), from: selection.from, to: selection.to, selected: view.state.sliceDoc(selection.from, selection.to) };
  });
}
async function ready() { await page.waitForSelector(editorSelector, { state: 'visible', timeout: 10000 }); }
async function focus() {
  await ready();
  await page.focus(editorSelector);
  await page.waitForFunction(() => document.activeElement?.matches('#editor .cm-content'), undefined, { timeout: 5000 });
}
async function textIs(expected) {
  // waitForFunction 保持同步，通过 DOM 等待编辑后再读取真实文档。
  for (let attempt = 0; attempt < 20; attempt++) {
    const actual = await state();
    if (actual.text === expected) return actual;
    if (attempt === 19) throw Error(`文档不一致：期望 ${JSON.stringify(expected)}，实际 ${JSON.stringify(actual)}`);
    await page.waitForTimeout(50);
  }
}
async function replaceDocument(text) {
  await focus(); await page.keyboard.press('ControlOrMeta+a');
  if (text) await page.keyboard.insertText(text); else await page.keyboard.press('Backspace');
  await textIs(text);
}
async function screenshot(name) {
  const file = path.join(output, `e2e-${name}.png`);
  await page.screenshot({ path: path.join(staging, path.basename(file)) }); result.artifacts.push(file);
}
async function check(name, action) {
  const entry = { name, status: 'passed' };
  try { const details = await action(); if (details !== undefined) entry.details = details; }
  catch (error) {
    // 功能断言失败记录证据并继续独立检查；控制权或运行时错误停止，不新建空间。
    entry.status = 'failed'; entry.error = String(error.stack ?? error);
    entry.snapshot = await page.snapshot();
    await screenshot(`failure-${result.checks.length + 1}`);
    if (/control|inactive|unassigned|executionStopped|mayHaveLateEffects/i.test(entry.error)) throw error;
  } finally { result.checks.push(entry); console.log(`${entry.status}：${name}`); }
}
async function more(label) {
  // 沿用户可见的更多入口操作，避免直接点击隐藏的常驻菜单按钮。
  await page.click('#more-actions');
  await page.waitForSelector('.action-menu', { state: 'visible' });
  await page.click(`loc=role:menuitem[name='${label}']`);
}
async function menu(label) {
  await more('格式与插入…');
  await page.waitForSelector('.action-menu', { state: 'visible' });
  await page.click(`loc=role:menuitem[name='${label}']`);
}
try {
  // 从同源静态资源备份存储，避免应用初始化先保存默认偏好。首次探索沿用当前页面。
  if (!globalThis.LEAFMARK_VERIFY_SPACE) await page.goto('http://localhost:5173/src/documents/session-recovery.ts');
  original = globalThis.LEAFMARK_VERIFY_ORIGINAL ?? await page.evaluate(keys => keys.map(key => [key, localStorage.getItem(key)]), keys);
  // 失败时仍保留准确备份供同一空间续跑；恢复完成后删除，避免保存用户偏好。
  await fs.writeFile(path.join(staging, 'e2e-storage-backup.json'), JSON.stringify({ spaceId: task.spaceId, entries: original }));
  await page.cdp('Runtime.enable');
  // 当前页面禁用开发热更新，避免其他代理编辑源码时重置正在验证的会话。
  await page.cdp('Page.addScriptToEvaluateOnNewDocument', { source: `
    const NativeWebSocket = window.WebSocket;
    window.WebSocket = class extends NativeWebSocket {
      constructor(url, protocols) {
        if (String(url).includes('localhost:5173') && protocols === 'vite-hmr') {
          const inert = new EventTarget();
          Object.assign(inert, { readyState: 0, send() {}, close() {} });
          return inert;
        }
        super(url, protocols);
      }
    };
  ` });
  // 只暂存本次会写入的三个键，绝不清理整个 origin 或共享 profile。
  await page.evaluate(() => {
    localStorage.setItem('leafmark-settings', JSON.stringify({ font: 'newsreader', fontSize: 19, lineHeight: 1.55, readingWidth: 'comfort', liveRendering: true, showActiveSyntax: true, autoSave: false, theme: 'light', focusMode: false, typewriter: false }));
    localStorage.setItem('leafmark-shortcuts-v1', JSON.stringify({ save: 'Mod-s', saveAs: 'Mod-Shift-s', open: 'Mod-o', new: 'Mod-n', close: 'Mod-w', find: 'Mod-f', bold: 'Mod-b', italic: 'Mod-i', link: 'Mod-k', reading: 'Mod-Shift-r', settings: 'Mod-,' }));
  });
  // 初次探索页面没有修改正文；重新载入使本次临时偏好生效。
  await page.goto(result.url); await ready();
  await page.waitForFunction(() => document.querySelector('dialog[open]') || document.querySelector('#editor .cm-content[contenteditable="true"]'), undefined, { timeout: 10000 });
  // 初始化恢复对话框在模块末尾才显示，等待浏览器完成当前任务再判断。
  await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
  if (await page.evaluate(() => [...document.querySelectorAll('dialog[open] button')].some(b => b.textContent === '稍后'))) await page.click('text="稍后"');
  await page.snapshot();
  await check('普通 MODE，无验证专用入口', async () => {
    assert(await page.evaluate(() => window.leafmarkVerification === undefined && window.editorVerification === undefined), '意外启用了验证构建入口');
    assert((await state()).text.length > 0, '编辑器 state 无法读取');
  });
  const chinese = '中文输入验证，叶笺让想法落在纸上。';
  let firstTab;
  await check('新建文档与中文输入', async () => {
    await page.click('#new-file'); await focus(); await page.keyboard.insertText(chinese); await textIs(chinese);
    firstTab = await page.evaluate(() => [...document.querySelectorAll('#tab-list .file-tab')].findIndex(e => e.classList.contains('active')));
    assert(await page.evaluate(() => !document.querySelector('#dirty-dot').hidden), '没有未保存标记');
  });
  await check('键盘选区与工具栏加粗', async () => {
    await focus(); await page.keyboard.press('ControlOrMeta+a');
    assert((await state()).selected === chinese, '全选没有覆盖真实文档');
    await page.waitForSelector('#format-toolbar:not([hidden])', { state: 'visible' });
    await page.snapshot(); await page.click('#format-toolbar button[data-format="bold"]');
    await textIs(`**${chinese}**`);
  });
  await check('源码切换保持正文', async () => {
    await page.click('#mode-toggle');
    assert(await page.evaluate(() => document.querySelector('#edit-mode').textContent === '源码编辑'), '没有进入源码模式');
    await textIs(`**${chinese}**`); await screenshot('source'); await page.click('#mode-toggle');
    await textIs(`**${chinese}**`);
  });
  await check('撤销与重做', async () => {
    await focus(); await page.keyboard.press('ControlOrMeta+z'); await textIs(chinese);
    await page.keyboard.press('ControlOrMeta+Shift+z'); await textIs(`**${chinese}**`);
  });
  await check('标签切换保留内容、选区与撤销栈', async () => {
    const before = await state(); await page.click('#new-file'); await focus();
    await page.keyboard.insertText('第二个标签的中文内容'); await textIs('第二个标签的中文内容');
    await page.click(`#tab-list .file-tab:nth-child(${firstTab + 1}) > button:first-child`);
    await ready(); const after = await textIs(before.text);
    assert(after.from === before.from && after.to === before.to, '标签切换丢失选区');
    await focus(); await page.keyboard.press('ControlOrMeta+z'); await textIs(chinese);
    await page.keyboard.press('ControlOrMeta+Shift+z'); await textIs(before.text);
  });
  await check('查找并全部替换中文', async () => {
    await replaceDocument('苹果 苹果\n香蕉'); await more('查找与替换');
    await page.snapshot(); await page.fill('input[name="search"]', '苹果');
    await page.fill('input[name="replace"]', '橙子'); await page.click('button[name="replaceAll"]');
    await textIs('橙子 橙子\n香蕉'); await page.click('button[name="close"]');
  });
  await check('表格插入与对话框编辑', async () => {
    await replaceDocument(''); await menu('插入表格');
    const inserted = '| 列 1 | 列 2 |\n| --- | --- |\n| 内容 | 内容 |'; await textIs(inserted);
    await focus(); await page.keyboard.press('ControlOrMeta+Home'); await menu('编辑当前表格…');
    await page.waitForSelector('dialog[aria-label="编辑表格"]', { state: 'visible' }); await page.snapshot();
    await page.fill('input[aria-label="表头，第 1 列"]', '项目');
    await page.fill('input[aria-label="第 1 行，第 1 列"]', '中文表格');
    // 列对齐已按原型改为按钮组，点击“居中”后再次点击同一按钮可恢复默认。
    await page.click('button[aria-label="居中"]');
    await page.click('dialog[aria-label="编辑表格"] button[type="submit"]');
    await textIs('| 项目 | 列 2 |\n| :---: | --- |\n| 中文表格 | 内容 |');
    await screenshot('table');
  });
  const documentText = '# 端到端验证\n\n**中文加粗**与正常正文。\n\n| 项目 | 状态 |\n| --- | --- |\n| 中文表格 | 已验证 |\n\n```mermaid\ngraph TD\n  A[中文输入] --> B[安全预览]\n```\n\n<script>window.__leafmarkUnsafe = 1</script>\n<img src=x onerror="window.__leafmarkUnsafe = 2">\n[危险链接](javascript:alert(1))\n';
  await check('阅读模式渲染标题、加粗与表格', async () => {
    await replaceDocument(documentText); await more('切换阅读模式');
    await page.waitForSelector('#reading-view:not([hidden]) table', { state: 'visible' });
    const rendered = await page.evaluate(() => ({ h1: document.querySelector('#reading-view h1')?.textContent, bold: document.querySelector('#reading-view strong')?.textContent, table: document.querySelector('#reading-view table')?.textContent }));
    assert(rendered.h1 === '端到端验证' && rendered.bold === '中文加粗' && rendered.table.includes('中文表格'), '阅读内容与源文不符');
    return rendered;
  });
  await check('Mermaid 渲染', async () => {
    await page.waitForSelector('#reading-view svg', { state: 'visible', timeout: 20000 });
    assert(await page.evaluate(() => document.querySelector('#reading-view svg').textContent.includes('安全预览')), 'Mermaid 缺少中文节点');
    await screenshot('reading');
  });
  await check('安全 HTML 预览', async () => {
    const security = await page.evaluate(() => {
      const container = document.querySelector('#reading-view');
      return { executed: window.__leafmarkUnsafe !== undefined, dangerousNodes: container.querySelectorAll('script,img[src="x"],[onerror],a[href^="javascript:"]').length, escaped: container.textContent.includes('<script>window.__leafmarkUnsafe = 1</script>') };
    });
    assert(!security.executed && security.dangerousNodes === 0 && security.escaped, '预览未安全转义 HTML'); return security;
  });
  await check('导出 HTML 下载产物', async () => {
    await more('文件操作与导出…'); await page.snapshot();
    const pending = page.waitForEvent('download', { timeout: 30000 });
    await page.click('loc=role:menuitem[name="导出 HTML"]');
    const download = await pending; const file = path.join(output, 'e2e-export.html');
    await download.saveAs(path.join(staging, path.basename(file))); result.artifacts.push(file);
    const html = await fs.readFile(path.join(staging, path.basename(file)), 'utf8');
    assert(html.startsWith('<!doctype html>') && html.includes('<svg') && html.includes('中文表格') && html.includes('&lt;script&gt;'), '导出产物缺少内容或图形');
    assert(!/<script[\s>]|onerror\s*=\s*["']|href=["']javascript:/i.test(html.replace(/&lt;[^<]*?&gt;/g, '')), '导出含可执行危险内容');
    return { suggestedFilename: download.suggestedFilename(), bytes: Buffer.byteLength(html), path: file };
  });
  await check('设置即时生效与 Escape 返回', async () => {
    if (await page.evaluate(() => !document.querySelector('#reading-view').hidden)) await more('切换阅读模式');
    await more('设置…'); await page.waitForSelector('dialog.leafmark-settings', { state: 'visible' }); await page.snapshot();
    // 主题在“外观”页以按钮组呈现，字体与阅读宽度在“编辑器”页。
    await page.click('loc=role:tab[name="外观"]');
    await page.click('loc=role:button[name="深色"]');
    await page.click('loc=role:tab[name="编辑器"]');
    await page.selectOption('dialog.leafmark-settings select >> nth=0', 'mono');
    await page.click('loc=role:button[name="窄"]');
    assert(await page.evaluate(() => document.documentElement.dataset.theme === 'dark' && document.documentElement.style.getPropertyValue('--reading-width') === '600px' && document.documentElement.style.getPropertyValue('--book').includes('Geist Mono')), '外观设置未生效');
    await screenshot('settings'); await page.keyboard.press('Escape'); await ready();
    assert(await page.evaluate(() => !document.querySelector('dialog[open]')), 'Escape 未关闭设置');
  });
  await check('浏览器文件菜单磁盘操作保持当前草稿', async () => {
    const before = await state();
    for (const label of ['检查磁盘修改', '从磁盘重新加载…']) {
      await more('文件操作与导出…'); await page.snapshot();
      await page.click(`loc=role:menuitem[name='${label}']`);
      await textIs(before.text);
      assert(await page.evaluate(() => !document.querySelector('dialog[open]')), '浏览器无磁盘路径时意外打开覆盖对话框');
    }
    return { limitation: '浏览器仅验证无原生磁盘路径时不覆盖草稿；真实磁盘检测和重载需桌面验证。' };
  });
  await check('快捷键自定义、保存与实际触发', async () => {
    await menu('自定义快捷键…'); await page.snapshot();
    await page.selectOption('#leafmark-shortcuts-action', 'bold');
    await page.click('.shortcut-recorder'); await page.keyboard.press('ControlOrMeta+Alt+b');
    await page.click('.shortcut-primary');
    assert(await page.evaluate(() => JSON.parse(localStorage.getItem('leafmark-shortcuts-v1')).bold === 'Mod-Alt-b'), '快捷键未保存');
    await page.keyboard.press('Escape'); await replaceDocument('快捷键验证');
    await page.keyboard.press('ControlOrMeta+a'); await page.keyboard.press('ControlOrMeta+Alt+b'); await textIs('**快捷键验证**');
    await page.keyboard.press('ControlOrMeta+Shift+r'); await page.waitForSelector('#reading-view:not([hidden]) strong', { state: 'visible' });
    await screenshot('shortcut');
  });
  result.runtimeErrors = (await page.events()).filter(e => e.method === 'Runtime.exceptionThrown' || e.method === 'Runtime.consoleAPICalled' && e.params?.type === 'error');
  result.checks.push({ name: '运行时异常与 console.error', status: result.runtimeErrors.length ? 'failed' : 'passed', details: result.runtimeErrors });
} catch (error) {
  fatal = error; result.fatalError = String(error.stack ?? error);
} finally {
  // 先停掉应用定时器与卸载处理，再从同源静态资源恢复原值，防止退出重新保存草稿。
  if (original && !fatal) {
    try {
      await page.goto('http://localhost:5173/src/documents/session-recovery.ts', { waitUntil: 'commit' });
      await page.evaluate(entries => {
        for (const [key, value] of entries) { if (value === null) localStorage.removeItem(key); else localStorage.setItem(key, value); }
      }, original);
      result.cleanup.storageRestored = await page.evaluate(entries => entries.every(([key, value]) => localStorage.getItem(key) === value), original);
      assert(result.cleanup.storageRestored, '本次涉及的存储条目未恢复');
      await fs.rm(path.join(staging, 'e2e-storage-backup.json'), { force: true });
      result.cleanup.finish = await task.finish({ keep: [] });
    } catch (error) { result.cleanup.error = String(error.stack ?? error); fatal = error; }
  }
  result.finishedAt = new Date().toISOString();
  result.passed = !fatal && result.checks.every(c => c.status === 'passed') && result.cleanup.storageRestored === true;
  for (const artifact of result.artifacts) await fs.copyFile(path.join(staging, path.basename(artifact)), artifact);
  if (fatal) result.cleanup.backupDirectory = staging;
  else await fs.rm(staging, { recursive: true, force: true });
  await fs.writeFile(path.join(output, 'e2e-results.json'), JSON.stringify(result, null, 2) + '\n');
}
console.log(JSON.stringify({ passed: result.passed, spaceId: task.spaceId, checks: result.checks.map(({ name, status }) => ({ name, status })), cleanup: result.cleanup, results: path.join(output, 'e2e-results.json') }, null, 2));
if (!result.passed) throw Error('端到端验证存在失败，详见 verification/e2e-results.json');
