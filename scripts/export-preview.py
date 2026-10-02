"""将前端 JS、Vite 样式和字体打包为可离线查看的单个 HTML 文件。"""
from pathlib import Path
import base64
import mimetypes
import re
import subprocess
import tempfile

root = Path(__file__).resolve().parent.parent
frontend = root / 'dist'
html = (frontend / 'index.html').read_text()

# Vite 成品含相对动态分块；重新合并 JS，避免 file:// 下发生模块加载失败。
with tempfile.TemporaryDirectory(prefix='leafmark-preview-') as temporary:
    bundle = Path(temporary) / 'bundle.js'
    subprocess.run(['bun', str(root / 'scripts/bundle-preview.ts'), str(bundle)], cwd=root, check=True)
    javascript = bundle.read_text()


def escape_closing_tag(text, tag):
    return re.sub(r'</' + tag, lambda match: '<\\/' + match.group(0)[2:], text, flags=re.IGNORECASE)


def inline_script(match):
    return '<script type="module">' + escape_closing_tag(javascript, 'script') + '</script>'


def read_css(path):
    css = path.read_text()

    def inline_asset(asset):
        url = asset.group(1).strip('"\'')
        if url.startswith(('data:', '#')):
            return asset.group(0)
        if re.match(r'(?:https?:)?//', url):
            raise ValueError(f'预览样式包含未内嵌的网络资源：{url}')
        resource = frontend / url.lstrip('/') if url.startswith('/') else path.parent / url
        resource = resource.resolve()
        if not resource.is_relative_to(frontend.resolve()):
            raise ValueError(f'样式资源位于构建目录之外：{resource}')
        mime = mimetypes.guess_type(resource.name)[0] or 'application/octet-stream'
        data = base64.b64encode(resource.read_bytes()).decode()
        return f'url("data:{mime};base64,{data}")'

    return re.sub(r'url\(([^)]+)\)', inline_asset, css)


# 所有动态 JS 已合并，预加载分块无需保留。
html = re.sub(r'<link\b[^>]*rel="modulepreload"[^>]*>', '', html)
html = re.sub(r'<link\b[^>]*rel="stylesheet"[^>]*>', '', html)
# 同时内嵌动态组件的 CSS，避免只收集 index.html 引用而漏掉分块样式。
styles = '\n'.join(read_css(path) for path in sorted(frontend.rglob('*.css')))
html = html.replace('</head>', '<style>' + escape_closing_tag(styles, 'style') + '</style></head>')
# 手机查看预览时优先显示正文；桌面应用保持原型的侧栏布局。
html = html.replace('</head>', '<style>@media(max-width:700px){.sidebar{display:none}.workspace{margin-left:0}.cm-scroller{padding:20px 14px 60px}.cm-line.md-h1{font-size:28px}.document-tab{max-width:55vw}.statusbar>div{gap:8px}#read-time{display:none}}</style></head>')
# 最后嵌入 JS，防止依赖源码中的 HTML 字面量被上面的标签替换规则误改。
html = re.sub(r'<script type="module"[^>]*src="([^"]+)"[^>]*></script>', inline_script, html)
output = root / 'Leafmark-preview.html'
output.write_text(html)
print(f'已生成单文件预览：{output}（{output.stat().st_size} bytes）')
