// 叶笺官网交互：无依赖、无构建步骤，全部为渐进增强。
(() => {
  const $ = (selector, root = document) => root.querySelector(selector);
  const $$ = (selector, root = document) => [...root.querySelectorAll(selector)];
  const reducedMotion = matchMedia("(prefers-reduced-motion: reduce)").matches;

  // 导航：离开页首后加上纸色底。
  const nav = $("#nav");
  const onScroll = () => nav.classList.toggle("scrolled", scrollY > 12);
  addEventListener("scroll", onScroll, { passive: true });
  onScroll();

  // 入场动效：同一行内的元素依次出现。
  const reveals = $$(".reveal");
  if ("IntersectionObserver" in window && !reducedMotion) {
    const observer = new IntersectionObserver(entries => {
      const visible = entries.filter(entry => entry.isIntersecting);
      visible.forEach((entry, index) => {
        entry.target.style.setProperty("--delay", `${index * 70}ms`);
        entry.target.classList.add("in");
        observer.unobserve(entry.target);
      });
    }, { rootMargin: "0px 0px -8% 0px", threshold: 0.08 });
    reveals.forEach(node => observer.observe(node));
  } else {
    reveals.forEach(node => node.classList.add("in"));
  }

  // 滚动动效：叶片视差、首屏窗口放平、通栏语法带随滚动推移；只在滚动时按帧更新。
  const drifting = $$("[data-parallax]");
  const tilting = $("#hero-window");
  const ribbon = $("#ribbon");
  const track = $(".ribbon-track", ribbon);
  if (!reducedMotion) {
    // 语法带复制两份首尾相接，循环滚动时不露出空白。
    const items = [...track.children];
    for (let copy = 0; copy < 2; copy++) items.forEach(item => track.append(item.cloneNode(true)));
    track.classList.add("run");
    let queued = false;
    const update = () => {
      queued = false;
      const middle = innerHeight / 2;
      for (const node of drifting) {
        const box = node.getBoundingClientRect();
        if (box.bottom < -200 || box.top > innerHeight + 200) continue;
        const current = Number(node.dataset.offset || 0);
        const offset = (box.top - current + box.height / 2 - middle) * -Number(node.dataset.parallax);
        node.dataset.offset = offset.toFixed(1);
        node.style.translate = `0 ${offset.toFixed(1)}px`;
      }
      // 窗口顶边从视口下沿升到三分之一处的过程中，由后仰逐渐放平。
      const top = tilting.getBoundingClientRect().top;
      const tilt = Math.min(1, Math.max(0, (top - innerHeight * 0.32) / (innerHeight * 0.6)));
      tilting.style.setProperty("--tilt", tilt.toFixed(3));
      const band = ribbon.getBoundingClientRect();
      if (band.bottom > 0 && band.top < innerHeight) track.style.translate = `${((band.top - innerHeight) * 0.16).toFixed(1)}px 0`;
    };
    addEventListener("scroll", () => { if (!queued) { queued = true; requestAnimationFrame(update); } }, { passive: true });
    addEventListener("resize", update);
    update();
  }

  // 首屏截图：浅色 / 深色切换。
  const heroWindow = $("#hero-window");
  $$("[data-set-theme]").forEach(button => button.addEventListener("click", () => {
    heroWindow.dataset.theme = button.dataset.setTheme;
    $$("[data-set-theme]").forEach(other => other.setAttribute("aria-pressed", String(other === button)));
  }));

  // 原位编辑演示：活动行显示语法标记，未操作时自动逐行移动。
  const demo = $("#demo");
  const lines = $$(".dl", demo);
  let autoplay = null;
  const activate = line => lines.forEach(other => other.classList.toggle("active", other === line));
  const stopAutoplay = () => { clearInterval(autoplay); autoplay = null; };
  lines.forEach(line => {
    line.tabIndex = 0;
    line.addEventListener("pointerdown", () => { stopAutoplay(); activate(line); });
    line.addEventListener("focus", () => { if (line.matches(":focus-visible")) { stopAutoplay(); activate(line); } });
  });
  $$("button[data-mode]", demo).forEach(button => button.addEventListener("click", () => {
    stopAutoplay();
    demo.dataset.mode = button.dataset.mode;
    $$("button[data-mode]", demo).forEach(other => other.setAttribute("aria-pressed", String(other === button)));
  }));
  const marked = lines.filter(line => $(".mk", line));
  activate(marked[0]);
  if (!reducedMotion && "IntersectionObserver" in window) {
    let step = 0;
    new IntersectionObserver(([entry], observer) => {
      if (!entry.isIntersecting) return;
      observer.disconnect();
      autoplay = setInterval(() => activate(marked[++step % marked.length]), 2200);
    }, { threshold: 0.4 }).observe(demo);
  }

  // 阅读模式：点击后方窗口或分段按钮，切换置于前方的主题。
  const stack = $("#stack");
  const bringToFront = layer => {
    stack.dataset.front = layer;
    $$("button[data-front]").forEach(button => button.setAttribute("aria-pressed", String(button.dataset.front === layer)));
  };
  $$("[data-layer]", stack).forEach(layer => layer.addEventListener("click", () => bringToFront(layer.dataset.layer)));
  $$("button[data-front]").forEach(button => button.addEventListener("click", () => bringToFront(button.dataset.front)));

  // 快捷键按当前平台显示修饰键。
  const platform = navigator.userAgentData?.platform || navigator.platform || "";
  const isMac = /mac|iphone|ipad/i.test(platform);
  const isWindows = /win/i.test(platform);
  $$("kbd[data-mod]").forEach(key => { key.textContent = isMac ? "⌘" : "Ctrl"; });
  $$("kbd[data-shift]").forEach(key => { key.textContent = isMac ? "⇧" : "Shift"; });
  $$("#demo code").forEach(code => { code.textContent = isMac ? "⌘⇧R" : "Ctrl+Shift+R"; });

  // 下载：标出适合当前设备的安装包。Safari 不暴露芯片架构，macOS 默认推荐 Apple 芯片。
  const recommend = arch => {
    const os = isMac ? "darwin" : isWindows ? "windows" : "";
    if (!os) return;
    const fallback = isMac ? "arm64" : "amd64";
    const card = $(`[data-asset="${os}-${arch || fallback}"]`);
    card?.classList.add("is-recommended");
  };
  if (navigator.userAgentData?.getHighEntropyValues) {
    navigator.userAgentData.getHighEntropyValues(["architecture"])
      .then(({ architecture }) => recommend(architecture === "arm" ? "arm64" : architecture === "x86" ? "amd64" : ""))
      .catch(() => recommend(""));
  } else {
    recommend("");
  }

  // 从最新发布读取版本号与安装包直链；请求失败时保留指向发布页的链接。
  const patterns = {
    "darwin-arm64": /darwin-arm64\.dmg$/,
    "darwin-amd64": /darwin-amd64\.dmg$/,
    "windows-amd64": /windows-amd64\.exe$/,
    "windows-arm64": /windows-arm64\.exe$/,
  };
  fetch("https://api.github.com/repos/KevinXC5/leafmark/releases/latest", { headers: { Accept: "application/vnd.github+json" } })
    .then(response => response.ok ? response.json() : Promise.reject(response.status))
    .then(release => {
      if (release.tag_name) $$("[data-version]").forEach(node => { node.textContent = release.tag_name; });
      for (const [key, pattern] of Object.entries(patterns)) {
        const asset = (release.assets || []).find(item => pattern.test(item.name));
        const card = $(`[data-asset="${key}"]`);
        if (!asset || !card) continue;
        card.href = asset.browser_download_url;
        card.removeAttribute("target");
        $(".dl-size", card).textContent = `${(asset.size / 1048576).toFixed(1)} MB`;
      }
    })
    .catch(() => {});
})();
