// 页面侧辅助函数：运行器把本文件原文拼在每个页面步骤之前，一起交给 WKWebView 执行。
// 这里的名称是步骤函数里的自由变量，不能在 Node 侧导入使用。
const T = window.leafmarkVerification;
const V = T.editor;
const F = "```";
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
const $ = selector => document.querySelector(selector);
const $$ = selector => [...document.querySelectorAll(selector)];
const until = async (ready, label) => {
  for (let i = 0; i < 80; i++) {
    if (ready()) return;
    await sleep(50);
  }
  throw Error("等待超时：" + label);
};
const eq = (actual, expected, label) => {
  if (actual !== expected) throw Error(label + " 不符：\n" + JSON.stringify(actual) + "\n期望 " + JSON.stringify(expected));
};
const css = name => getComputedStyle(document.documentElement).getPropertyValue(name).trim();
const toast = () => ($("#toast").hidden ? "" : $("#toast").textContent);

// 编辑器
const doc = () => V.state.doc.toString();
const setDoc = (text, anchor = 0) => V.dispatch({ changes: { from: 0, to: V.state.doc.length, insert: text }, selection: { anchor } });
const blur = async () => {
  V.contentDOM.blur();
  $("#documents-tab").focus();
  await sleep(150);
};
const key = (name, options = {}) => V.contentDOM.dispatchEvent(new KeyboardEvent("keydown", { key: name, bubbles: true, cancelable: true, ...options }));
const menu = async (toggle, label) => {
  $(toggle).click();
  await sleep(40);
  const button = $$(".action-menu button").find(item => item.textContent === label);
  if (!button) throw Error("无菜单项 " + label);
  button.click();
  await sleep(150);
};
const setTheme = async theme => {
  if (document.documentElement.dataset.theme !== theme) {
    $("#theme-toggle").click();
    await sleep(150);
  }
};

// 文件导航
const rows = () => $$(".fb-tree .fb-node-open").map(button => button.textContent.trim());
const node = name => $$(".fb-tree .fb-node-open").find(button => button.textContent.trim() === name);
const idle = () => until(() => $(".fb-browser").getAttribute("aria-busy") === "false", "文件导航空闲");
const setInput = (input, value) => {
  input.value = value;
  input.dispatchEvent(new Event("input", { bubbles: true }));
};
const wsMenu = async label => {
  $("#documents-tab").click();
  await idle();
  $(".fb-toolbar .fb-more").click();
  await sleep(50);
  const item = $$(".fb-menu [role=menuitem]").find(button => button.textContent === label);
  if (!item) throw Error("菜单缺少：" + label);
  item.click();
  await sleep(80);
};
// 原生“选择文件夹”对话框无法自动化：后端先切换工作区，这里从界面菜单刷新。
const refreshWorkspace = async name => {
  await wsMenu("刷新工作区和近期文件");
  await until(() => $(".fb-workspace-choose").textContent === name, "工作区 " + name);
  await idle();
};
const openFile = async (name, folder) => {
  $("#documents-tab").click();
  await idle();
  if (!node(name) && folder) {
    node(folder).click();
    await sleep(150);
  }
  node(name).click();
  await until(() => T.document().name === name, "打开 " + name);
  await idle();
  await sleep(150);
};
const newFile = async name => {
  await wsMenu("在此新建文件…");
  const dialog = $("dialog.fb-dialog");
  dialog.querySelector("input").value = name;
  dialog.querySelector("form").requestSubmit();
  await until(() => T.document().name === name, "新建 " + name);
  await idle();
  await sleep(150);
};

// 设置页
const S = () => $(".leafmark-settings");
const stored = () => JSON.parse(localStorage.getItem("leafmark-settings") || "null");
const openS = async () => {
  if (!S()) {
    $("#settings-toggle").click();
    await sleep(80);
  }
};
const closeS = async () => {
  if (S()) {
    $(".settings-close").click();
    await sleep(80);
  }
};
const tab = async name => {
  $$(".settings-tab")
    .find(item => item.textContent.includes(name))
    .click();
  await sleep(60);
};
const panel = () => $(".settings-panel:not([hidden])");
const rowOf = label => $$(".settings-row").find(row => row.querySelector(".settings-label")?.textContent === label);
const change = (input, value) => {
  input.value = value;
  input.dispatchEvent(new Event("change", { bubbles: true }));
};
const seg = (label, text) => [...rowOf(label).querySelectorAll("button")].find(button => button.textContent === text);
const resetSettings = async () => {
  await openS();
  $(".settings-footer .settings-reset").click();
  await sleep(80);
  const settings = stored();
  await closeS();
  return settings;
};

// 表格编辑器
const D = () => $("dialog.lm-table-dialog");
const tb = label => [...D().querySelectorAll("button")].find(button => button.textContent === label || button.getAttribute("aria-label") === label);
const cell = (row, column) => D().querySelector('input[data-row="' + row + '"][data-column="' + column + '"]');
const type = (input, value) => {
  input.focus();
  input.value = value;
  input.dispatchEvent(new Event("input", { bubbles: true }));
};
