/* MrRSS 布局原型（mrrss-280.5）。一次性原型代码：纯静态、无构建、无外部依赖。 */
(function () {
  'use strict';

  var D = window.PROTO_DATA;
  var initialRead = {};
  D.articles.forEach(function (a) { initialRead[a.id] = a.read; });

  // ---------------- 图标 ----------------
  var PATHS = {
    tray: '<path d="M3 13h5l1.5 3h5L16 13h5"/><path d="M5 5h14l2 8v6H3v-6z"/>',
    list: '<path d="M8 6h13M8 12h13M8 18h13"/><circle cx="4" cy="6" r=".8"/><circle cx="4" cy="12" r=".8"/><circle cx="4" cy="18" r=".8"/>',
    gear: '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z"/>',
    ext: '<path d="M14 4h6v6"/><path d="M20 4l-9 9"/><path d="M18 14v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V7a1 1 0 0 1 1-1h5"/>',
    spark: '<path d="M12 3l1.8 4.9L19 9.7l-5.2 1.8L12 16.5l-1.8-5L5 9.7l5.2-1.8z"/><path d="M19 15l.8 2.2L22 18l-2.2.8L19 21l-.8-2.2L16 18l2.2-.8z"/>',
    mailOpen: '<path d="M3 10l9-6 9 6v9a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1z"/><path d="M3 10l9 6 9-6"/>',
    mail: '<rect x="3" y="5" width="18" height="14" rx="1"/><path d="M3 7l9 6 9-6"/>',
    refresh: '<path d="M20 11a8 8 0 0 0-14.3-4.9L4 8"/><path d="M4 4v4h4"/><path d="M4 13a8 8 0 0 0 14.3 4.9L20 16"/><path d="M20 20v-4h-4"/>',
    chev: '<path d="M6 9l6 6 6-6"/>',
    x: '<path d="M6 6l12 12M18 6L6 18"/>',
    zin: '<circle cx="11" cy="11" r="7"/><path d="M21 21l-4.3-4.3M8 11h6M11 8v6"/>',
    zout: '<circle cx="11" cy="11" r="7"/><path d="M21 21l-4.3-4.3M8 11h6"/>',
    left: '<path d="M15 6l-6 6 6 6"/>',
    right: '<path d="M9 6l6 6-6 6"/>',
    up: '<path d="M12 19V5M6 11l6-6 6 6"/>',
    down: '<path d="M12 5v14M6 13l6 6 6-6"/>',
    check: '<path d="M5 12l5 5L20 7"/>',
    checks: '<path d="M2 12l5 5L17 7"/><path d="M12 16l1 1L23 7"/>',
    warn: '<path d="M12 3l10 18H2z"/><path d="M12 10v5M12 18v.01"/>',
    all: '<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M3 9h18"/>',
    cloud: '<path d="M7 18a5 5 0 1 1 .9-9.9A6 6 0 0 1 19 10a4 4 0 0 1-1 8z"/>'
  };
  function icon(n, cls) { return '<svg class="i ' + (cls || '') + '" viewBox="0 0 24 24">' + (PATHS[n] || '') + '</svg>'; }
  function esc(s) { return String(s == null ? '' : s).replace(/[&<>"']/g, function (c) { return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]; }); }

  // ---------------- 状态 ----------------
  var VARIANTS = ['A', 'B', 'C'];
  function loadVariant() {
    var h = (location.hash || '').replace('#', '').toUpperCase();
    if (VARIANTS.indexOf(h) >= 0) return h;
    try { var v = localStorage.getItem('mrrss-proto-variant'); if (VARIANTS.indexOf(v) >= 0) return v; } catch (e) {}
    return 'A';
  }
  var S = {
    variant: loadVariant(),
    view: 'unread',
    scope: { type: 'all', id: null },
    snapshot: [],
    selected: null,
    fetch: {},    // id -> fetching | ok | fail
    sum: {},      // id -> loading | done
    collapsed: {},
    syncing: false,
    lastSyncMin: 3,
    menu: null,
    viewer: null,
    toast: null,
    settings: false,
    notes: false,
    newAdded: 0
  };
  try { S.notes = localStorage.getItem('mrrss-proto-notes') === '1'; } catch (e) {}

  function feedById(id) { for (var i = 0; i < D.feeds.length; i++) if (D.feeds[i].id === id) return D.feeds[i]; }
  function catById(id) { for (var i = 0; i < D.categories.length; i++) if (D.categories[i].id === id) return D.categories[i]; }
  function artById(id) { for (var i = 0; i < D.articles.length; i++) if (D.articles[i].id === id) return D.articles[i]; }

  function inScope(a, scope) {
    scope = scope || S.scope;
    if (scope.type === 'all') return true;
    if (scope.type === 'feed') return a.feed === scope.id;
    if (scope.type === 'cat') return feedById(a.feed).cat === scope.id;
    return false;
  }
  function unreadIn(scope) { return D.articles.filter(function (a) { return !a.read && inScope(a, scope); }).length; }
  function scopeName(scope) {
    scope = scope || S.scope;
    if (scope.type === 'all') return '全部订阅';
    if (scope.type === 'feed') return feedById(scope.id).name;
    return catById(scope.id).name;
  }
  function takeSnapshot() {
    S.snapshot = D.articles
      .filter(function (a) { return inScope(a) && (S.view === 'all' || !a.read); })
      .sort(function (a, b) { return a.minutes - b.minutes; })
      .map(function (a) { return a.id; });
  }
  function pendingNew() {
    // 快照之后同步进来、属于当前范围且未读的文章（sync-model：列表不自动替换，只提示）
    return D.articles.filter(function (a) { return a.isNew && inScope(a) && S.snapshot.indexOf(a.id) < 0 && !a.read; }).length;
  }
  function rel(min) {
    if (min < 1) return '刚刚';
    if (min < 60) return min + ' 分钟前';
    if (min < 1440) return Math.floor(min / 60) + ' 小时前';
    return Math.floor(min / 1440) + ' 天前';
  }
  function shortTime(min) {
    if (min < 60) return min + '分';
    if (min < 1440) return Math.floor(min / 60) + '时';
    return Math.floor(min / 1440) + '天';
  }
  function fav(feed) { return '<span class="fav" style="background:' + feed.color + '">' + esc(feed.name.charAt(0)) + '</span>'; }

  // ---------------- 行为 ----------------
  function setScope(type, id) { S.scope = { type: type, id: id }; S.selected = null; takeSnapshot(); render(); }
  function setView(v) { S.view = v; S.selected = null; takeSnapshot(); render(); }

  function openArticle(id) {
    var a = artById(id);
    S.selected = id;
    a.read = true; // 点开即已读
    if (a.kind !== 'full' && !S.fetch[id]) {
      S.fetch[id] = 'fetching';
      setTimeout(function () {
        S.fetch[id] = a.kind === 'trunc-ok' ? 'ok' : 'fail';
        render();
      }, 1100);
    }
    render(true);
  }
  function summarize(id) {
    var a = artById(id);
    if (!canSummarize(a)) return;
    S.sum[id] = 'loading';
    render();
    var wait = S.fetch[id] === 'fetching' ? 2200 : 1400;
    setTimeout(function () {
      // 全文抓取失败且 RSS 太短：不生成摘要，失败提示已经说明原因（D2）
      if (!canSummarize(a)) delete S.sum[id]; else S.sum[id] = 'done';
      render();
    }, wait);
  }
  function openInBrowser(id) {
    var a = artById(id);
    toast('（原型）已在系统浏览器打开：' + a.url);
  }
  function setRead(ids, val, label) {
    var changed = ids.filter(function (id) { return artById(id).read !== val; });
    changed.forEach(function (id) { artById(id).read = val; });
    if (label) {
      toast(label.replace('{n}', changed.length), changed.length ? function () {
        changed.forEach(function (id) { artById(id).read = !val; });
        render();
      } : null);
    }
    render();
  }
  function markScope(scope) {
    var ids = D.articles.filter(function (a) { return inScope(a, scope) && !a.read; }).map(function (a) { return a.id; });
    setRead(ids, true, '「' + scopeName(scope) + '」{n} 篇已标为已读');
  }
  function markAboveBelow(id, dir) {
    var i = S.snapshot.indexOf(id);
    var ids = dir === 'above' ? S.snapshot.slice(0, i + 1) : S.snapshot.slice(i);
    setRead(ids, true, (dir === 'above' ? '此篇及以上' : '此篇及以下') + ' {n} 篇已标为已读');
  }
  var toastTimer = null;
  function toast(text, undo) {
    S.toast = { text: text, undo: undo };
    clearTimeout(toastTimer);
    toastTimer = setTimeout(function () { S.toast = null; render(); }, 4500);
    render();
  }
  var newSeq = 0;
  var NEW_POOL = [
    ['f-36kr', '9 点 1 氪｜某头部平台发布三季度财报预告'],
    ['f-ithome', 'Chrome 142 正式版发布：标签页搜索全面上线'],
    ['f-verge', '谷歌展示新一代智能眼镜原型', 'Google shows off its next smart glasses prototype'],
    ['f-36kr', '某新能源车企宣布海外工厂投产'],
    ['f-ruanyf', '科技爱好者周刊（第 369 期）：小工具的胜利']
  ];
  function simulateSync(userInitiated) {
    if (S.syncing) return;
    S.syncing = true; render();
    setTimeout(function () {
      S.syncing = false; S.lastSyncMin = 0;
      if (!userInitiated) {
        for (var k = 0; k < 3; k++) {
          var x = NEW_POOL[newSeq++ % NEW_POOL.length];
          D.articles.push({
            id: 'n' + newSeq, feed: x[0], title: x[1], origTitle: x[2], lang: x[2] ? 'en' : 'zh', minutes: 0, read: false, isNew: true,
            kind: 'full', images: [], url: feedById(x[0]).site + '/n' + newSeq,
            body: '<p>（同步后新到的文章。列表不会自动插入它们，只在顶部提示，点击后才重新取快照。）</p>', snippet: '同步后新到的文章。'
          });
        }
        D.articles.forEach(function (a) { if (a.isNew && !a._stamped) { a._stamped = true; } else a.minutes += 1; });
      } else {
        takeSnapshot(); // 用户主动刷新：重新取快照
      }
      render();
    }, 1000);
  }
  function resetAll() {
    D.articles = D.articles.filter(function (a) { return !a.isNew; });
    D.articles.forEach(function (a) { a.read = initialRead[a.id]; });
    S.fetch = {}; S.sum = {}; S.selected = null; S.view = 'unread'; S.scope = { type: 'all', id: null };
    takeSnapshot(); render();
  }

  // ---------------- 公共片段 ----------------
  function tree() {
    var h = '';
    var allCnt = unreadIn({ type: 'all' });
    h += nodeHtml('all', null, icon('all'), '全部订阅', allCnt, 'top');
    h += '<div class="sb-sep"></div>';
    D.categories.forEach(function (c) {
      var sc = { type: 'cat', id: c.id };
      var closed = !!S.collapsed[c.id];
      h += nodeHtml('cat', c.id, '<span data-act="toggle-cat" data-id="' + c.id + '">' + icon('chev', 'chev' + (closed ? ' closed' : '')) + '</span>', c.name, unreadIn(sc), 'cat');
      if (!closed) D.feeds.filter(function (f) { return f.cat === c.id; }).forEach(function (f) {
        h += nodeHtml('feed', f.id, fav(f), f.name, unreadIn({ type: 'feed', id: f.id }), 'feed');
      });
    });
    D.feeds.filter(function (f) { return !f.cat; }).forEach(function (f) {
      h += nodeHtml('feed', f.id, fav(f), f.name, unreadIn({ type: 'feed', id: f.id }), 'feed top');
    });
    return '<div class="tree">' + h + '</div>';
  }
  function nodeHtml(type, id, lead, name, cnt, cls) {
    var on = S.scope.type === type && S.scope.id === id;
    return '<div class="node ' + cls + (on ? ' on' : '') + (cnt ? '' : ' dim') + '" data-act="scope" data-ctx="scope" data-type="' + type + '" data-id="' + (id || '') + '">' +
      lead + '<span class="name">' + esc(name) + '</span>' + (cnt ? '<span class="cnt">' + cnt + '</span>' : '') + '</div>';
  }
  function syncText() {
    if (S.syncing) return '正在同步…';
    return S.lastSyncMin === 0 ? '刚刚已同步' : S.lastSyncMin + ' 分钟前已同步';
  }
  function newBanner() {
    var n = pendingNew();
    return n ? '<div class="new-banner" data-act="load-new">↑ ' + n + ' 篇新文章，点击载入</div>' : '';
  }
  function listBody(style) {
    var rows = S.snapshot.map(function (id) { return rowHtml(artById(id), style); }).join('');
    if (!S.snapshot.length) return newBanner() + '<div class="empty">' + (S.view === 'unread' ? '这里没有未读文章了' : '没有文章') + '</div>';
    return newBanner() + rows + '<div class="list-end">— 已到底：共 ' + S.snapshot.length + ' 篇 —</div>';
  }
  function rowHtml(a, style) {
    var f = feedById(a.feed);
    var cls = 'row ' + style + (a.read ? ' read' : '') + (S.selected === a.id ? ' on' : '');
    var attrs = ' data-act="open" data-ctx="article" data-id="' + a.id + '"';
    if (style === 'compact') {
      return '<div class="' + cls + '"' + attrs + '><span class="dot"></span>' + fav(f) + '<span class="t">' + esc(a.title) + '</span><span class="src">' + esc(f.name) + '</span><span class="tm">' + shortTime(a.minutes) + '</span></div>';
    }
    if (style === 'comfy') {
      var thumb = a.images.length ? '<img class="thumb" src="' + a.images[0] + '" alt="">' : '';
      var sn = a.lang === 'zh' && a.snippet ? '<div class="sn">' + esc(a.snippet) + '</div>' : '';
      return '<div class="' + cls + '"' + attrs + '><span class="dot"></span><div class="body"><div class="meta">' + fav(f) + esc(f.name) + ' · ' + rel(a.minutes) + (a.lang === 'en' ? ' <span class="chip">译</span>' : '') + '</div><div class="t">' + esc(a.title) + '</div>' + sn + '</div>' + thumb + '</div>';
    }
    return '<div class="' + cls + '"' + attrs + '><span class="dot"></span><div class="body"><div class="t">' + esc(a.title) + '</div><div class="meta">' + fav(f) + esc(f.name) + ' · ' + rel(a.minutes) + '</div></div></div>';
  }

  // 正文与状态提示：三个变体共用同一套内容规则（D2/U4），只换动作按钮的位置
  function articleContent(a) {
    var st = S.fetch[a.id];
    var h = '';
    if (a.kind !== 'full') {
      if (st === 'fetching') h += '<div class="notice info">' + icon('refresh', 'spin') + '<div class="grow">RSS 里的正文被截断，正在抓取全文…</div></div>';
      else if (st === 'ok') h += '';
      else if (st === 'fail' && a.kind === 'trunc-fail-long') h += '<div class="notice warn">' + icon('warn') + '<div class="grow">' + esc(a.failReason) + '。下面显示的是 RSS 提供的摘录。</div><button class="btn" data-act="browser" data-id="' + a.id + '">' + icon('ext') + '在浏览器打开</button></div>';
      else if (st === 'fail') h += '<div class="notice warn">' + icon('warn') + '<div class="grow">' + esc(a.failReason) + '。RSS 只提供了很少的内容，无法生成摘要。</div><button class="btn primary" data-act="browser" data-id="' + a.id + '">' + icon('ext') + '在浏览器打开</button></div>';
    }
    h += summaryHtml(a);
    var body = a.kind === 'trunc-ok' && st === 'ok' ? a.full : a.body;
    h += '<div class="article-body" data-imgs="' + a.id + '">' + body + '</div>';
    return h;
  }
  function summaryHtml(a) {
    var s = S.sum[a.id];
    if (!s) return '';
    if (s === 'loading') {
      var wait = S.fetch[a.id] === 'fetching' ? '等待全文抓取完成后生成摘要…' : '正在生成摘要…';
      return '<div class="summary"><div class="h">' + icon('spark') + 'AI 摘要</div><div style="font-size:13px;color:var(--text-3)">' + wait + '</div><div class="skeleton" style="width:92%"></div><div class="skeleton" style="width:78%"></div><div class="skeleton" style="width:60%"></div></div>';
    }
    var lines = a.summary || ['这篇文章的要点一（原型占位）。', '要点二：摘要以三到五条要点呈现。', '要点三：点「在浏览器打开」可继续细读。'];
    var note = S.fetch[a.id] === 'fail' ? '<div class="note">全文抓取失败，本摘要基于 RSS 摘录生成，可能不完整。</div>' : '';
    return '<div class="summary"><div class="h">' + icon('spark') + 'AI 摘要</div><ul>' + lines.map(function (l) { return '<li>' + esc(l) + '</li>'; }).join('') + '</ul>' + note + '</div>';
  }
  function canSummarize(a) { return !(a.kind === 'trunc-fail-short' && S.fetch[a.id] === 'fail'); }
  function sumBtn(a, cls) {
    var dis = !canSummarize(a) || S.sum[a.id] === 'loading' || S.sum[a.id] === 'done';
    var label = S.sum[a.id] === 'loading' ? '生成中…' : S.sum[a.id] === 'done' ? '已摘要' : '摘要';
    var title = !canSummarize(a) ? ' title="正文太短，无法摘要"' : '';
    return '<button class="btn ' + (cls || '') + '" data-act="summary" data-id="' + a.id + '"' + (dis ? ' disabled' : '') + title + '>' + icon('spark') + label + '</button>';
  }
  function browserBtn(a, cls) { return '<button class="btn ' + (cls || '') + '" data-act="browser" data-id="' + a.id + '">' + icon('ext') + '在浏览器打开</button>'; }
  function readBtn(a, cls) {
    return '<button class="btn ' + (cls || '') + '" data-act="toggle-read" data-id="' + a.id + '">' + icon(a.read ? 'mail' : 'mailOpen') + (a.read ? '标为未读' : '标为已读') + '</button>';
  }
  function titleBlock(a) {
    var f = feedById(a.feed);
    return '<div class="src-line">' + fav(f) + esc(f.name) + ' · ' + rel(a.minutes) + '</div>' +
      '<h1>' + esc(a.title) + '</h1>' + (a.origTitle ? '<div class="orig">' + esc(a.origTitle) + '</div>' : '');
  }

  // ---------------- 变体 A：ActivityBar 精简（最接近现版本） ----------------
  function variantA() {
    var a = S.selected && artById(S.selected);
    var allUnread = unreadIn({ type: 'all' });
    var ab = '<div class="col activity">' +
      '<button class="ab-btn' + (S.view === 'unread' ? ' on' : '') + '" data-act="view" data-v="unread" title="未读">' + icon('tray') + (allUnread ? '<span class="ab-badge">' + allUnread + '</span>' : '') + '</button>' +
      '<button class="ab-btn' + (S.view === 'all' ? ' on' : '') + '" data-act="view" data-v="all" title="全部（含已读）">' + icon('list') + '</button>' +
      '<div style="flex:1"></div>' +
      '<button class="ab-btn" data-act="settings" title="设置">' + icon('gear') + '</button></div>';
    var sb = '<div class="col sidebar"><div class="sb-head">订阅</div><div class="scroll">' + tree() + '</div></div>';
    var list = '<div class="col list"><div class="list-head"><span class="title">' + esc(scopeName()) + '</span><span class="sub">' + (S.view === 'unread' ? '未读' : '全部') + ' · ' + S.snapshot.length + '</span><span class="grow"></span>' +
      '<span class="sub">' + syncText() + '</span><button class="icon-btn" data-act="refresh" title="立即同步">' + icon('refresh', S.syncing ? 'spin' : '') + '</button></div>' +
      '<div class="scroll js-list">' + listBody('two') + '</div></div>';
    var det = '<div class="col detail">' + (a ?
      '<div class="detail-bar"><span class="grow"></span>' + sumBtn(a) + browserBtn(a) + readBtn(a, 'ghost') + '</div>' +
      '<div class="scroll js-detail"><div class="reader">' + titleBlock(a) + '<hr>' + articleContent(a) + '</div></div>'
      : '<div class="placeholder">从左侧列表选一篇文章</div>') + '</div>';
    return ab + sb + list + det;
  }

  // ---------------- 变体 B：侧栏顶部分段切换 + 单行紧凑列表 + 底部浮动操作条 ----------------
  function variantB() {
    var a = S.selected && artById(S.selected);
    var allUnread = unreadIn({ type: 'all' });
    var sb = '<div class="col sidebar">' +
      '<div class="seg"><button class="' + (S.view === 'unread' ? 'on' : '') + '" data-act="view" data-v="unread">未读 <span class="n">' + allUnread + '</span></button>' +
      '<button class="' + (S.view === 'all' ? 'on' : '') + '" data-act="view" data-v="all">全部</button></div>' +
      '<div class="scroll">' + tree() + '</div>' +
      '<div class="sb-foot"><button class="sync-link" data-act="refresh" title="立即同步">' + icon('refresh', S.syncing ? 'spin' : '') + syncText() + '</button><span class="grow"></span>' +
      '<button class="icon-btn" data-act="settings" title="设置">' + icon('gear') + '</button></div></div>';
    var list = '<div class="col list wide"><div class="scope-line"><b>' + esc(scopeName()) + '</b> · ' + (S.view === 'unread' ? '未读' : '全部') + ' ' + S.snapshot.length + ' 篇</div>' +
      '<div class="scroll js-list">' + listBody('compact') + '</div></div>';
    var det = '<div class="col detail">' + (a ?
      '<div class="scroll js-detail"><div class="reader">' + titleBlock(a) + '<hr>' + articleContent(a) + '</div></div>' +
      '<div class="float-bar">' + sumBtn(a, 'primary') + browserBtn(a) + '<span class="div"></span>' + readBtn(a, 'ghost') + '</div>'
      : '<div class="placeholder">从列表选一篇文章</div>') + '</div>';
    return sb + list + det;
  }

  // ---------------- 变体 C：列表头标签 + 宽松列表 + 标题下行内操作 ----------------
  function variantC() {
    var a = S.selected && artById(S.selected);
    var scUnread = unreadIn(S.scope);
    var sb = '<div class="col sidebar"><div class="sb-head"><span class="grow">MrRSS</span><button class="icon-btn" data-act="settings" title="设置">' + icon('gear') + '</button></div>' +
      '<div class="scroll">' + tree() + '</div></div>';
    var list = '<div class="col list"><div class="list-head" style="padding-left:6px">' +
      '<div class="tabs"><button class="' + (S.view === 'unread' ? 'on' : '') + '" data-act="view" data-v="unread">未读<span class="n">' + scUnread + '</span></button>' +
      '<button class="' + (S.view === 'all' ? 'on' : '') + '" data-act="view" data-v="all">全部</button></div>' +
      '<span class="grow"></span><button class="icon-btn" data-act="refresh" title="' + syncText() + '，点击立即同步">' + icon(S.syncing ? 'refresh' : 'cloud', S.syncing ? 'spin' : '') + '</button></div>' +
      '<div class="scope-line"><b>' + esc(scopeName()) + '</b></div>' +
      '<div class="scroll js-list">' + listBody('comfy') + '</div></div>';
    var det = '<div class="col detail">' + (a ?
      '<div class="scroll js-detail"><div class="reader">' + titleBlock(a) +
      '<div class="inline-actions">' + sumBtn(a, 'primary') + browserBtn(a) + readBtn(a, 'ghost') + '</div><hr>' + articleContent(a) + '</div></div>'
      : '<div class="placeholder">从列表选一篇文章</div>') + '</div>';
    return sb + list + det;
  }

  // ---------------- 原型切换条与说明（不是设计的一部分） ----------------
  var NOTES = {
    A: {
      name: 'A · ActivityBar 精简',
      diff: [
        '保留现版本的四栏结构：ActivityBar ｜ 订阅侧栏 ｜ 文章列表 ｜ 详情。',
        'ActivityBar 只剩「未读」「全部」两个图标和底部「设置」；删掉收藏、稍后读、折叠侧栏、折叠 ActivityBar 四个按钮。未读图标上带总未读数。',
        '列表头只剩同步状态和一个「立即同步」按钮；删掉「全部标为已读」「只看未读」开关、筛选器。',
        '列表行：两行标题 + 源与时间，不带缩略图（现版本默认带预览图）。',
        '详情工具栏从 8 个按钮减到 3 个：摘要 / 在浏览器打开 / 标为未读；删掉关闭、译文切换、收藏、稍后读、复制链接、重新加载，以及上一篇/下一篇。',
        '英文文章：标题显示中文译文，原文标题放在详情标题下方（D1）。'
      ]
    },
    B: {
      name: 'B · 分段切换 + 紧凑列表 + 浮动操作条',
      diff: [
        '去掉 ActivityBar，变成三栏。「未读 / 全部」改成侧栏顶部的分段切换（macOS / iOS 常见的做法），设置移到侧栏左下角，和同步状态放一行。',
        '同步状态本身就是「立即同步」按钮，列表头不再有按钮。',
        '列表改成单行紧凑行（标题 · 源 · 时间），一屏能扫的条数约为 A 的 2 倍，适合 36氪这类量大的源。',
        '详情区没有顶部工具栏；三个动作放在正文底部居中的浮动条里，「摘要」是主按钮。正文从顶部直接开始。'
      ]
    },
    C: {
      name: 'C · 列表头标签 + 宽松列表 + 行内操作',
      diff: [
        '去掉 ActivityBar，三栏。「未读 / 全部」改成列表头的标签页（带当前范围的未读数），切换和它影响的列表放在一起。设置放在侧栏标题右边。',
        '同步状态压成列表头右上角的一个云图标，悬停看时间，点击立即同步。',
        '列表行最宽松：源与时间 → 两行标题 → 一行中文摘录，右侧缩略图；英文文章不显示英文摘录，只打「译」标记。',
        '详情区没有工具栏；操作按钮放在标题正下方（看完标题就决定：摘要还是去浏览器），「摘要」是主按钮。'
      ]
    }
  };
  function protoBar() {
    return '<div class="proto-bar"><span class="tag">原型切换条 · 不是设计的一部分 ｜</span>' +
      VARIANTS.map(function (v) { return '<button data-act="variant" data-v="' + v + '" class="' + (S.variant === v ? 'on' : '') + '">' + esc(NOTES[v].name) + '</button>'; }).join('') +
      '<span class="sp"></span>' +
      '<button data-act="sim-sync">模拟后台同步到 3 篇新文章</button>' +
      '<button data-act="reset">重置数据</button>' +
      '<button data-act="notes" class="' + (S.notes ? 'on' : '') + '">说明与评审要点</button></div>';
  }
  function notesPanel() {
    if (!S.notes) return '';
    var n = NOTES[S.variant];
    return '<div class="proto-notes">' +
      '<h3>当前：' + esc(n.name) + ' — 与现版本的区别</h3><ul>' + n.diff.map(function (d) { return '<li>' + esc(d) + '</li>'; }).join('') + '</ul>' +
      '<h3>三个变体都一样的部分</h3><ul>' +
      '<li>侧栏：全部订阅 + 分类（可折叠）→ 订阅源，右侧是未读数；无分类的源排在最后。</li>' +
      '<li>点开文章即标为已读。「未读」视图里，已读的文章变灰但不消失，直到切换范围或刷新（sync-model 的列表快照）。</li>' +
      '<li>后台同步到新文章时不插入列表，只在顶部提示「N 篇新文章」。</li>' +
      '<li>截断正文：点开自动抓全文并替换；失败时显示原因 +「在浏览器打开」（U4 / D2）。</li>' +
      '<li>文章右键：标为已读/未读、此篇及以上标为已读、此篇及以下标为已读、在浏览器打开。</li>' +
      '<li>源 / 分类 / 全部订阅右键：全部标为已读（批量操作后提示可撤销）。</li>' +
      '<li>点正文图片打开图片查看器：左右切换、缩放（按钮或滚轮）、Esc 或点背景关闭。</li>' +
      '</ul>' +
      '<h3>写死的正文排版（U9）</h3><ul>' +
      '<li>字体：系统无衬线（Windows 为 Segoe UI + 微软雅黑，macOS 为苹方）</li>' +
      '<li>正文 17px，行高 1.8，段间距 1em，最大行宽 700px（约 41 个汉字）</li>' +
      '<li>文章标题 26px / 1.4 粗体；小标题 1.2em；图注 13px</li>' +
      '</ul>' +
      '<h3>建议按这几条试</h3><ul>' +
      '<li>在「全部订阅」扫一屏标题，右键某篇 →「此篇及以上标为已读」。三种列表密度哪种扫得最快？</li>' +
      '<li>点开「少数派」那篇（截断 → 自动抓全文成功）、The Verge（抓取失败但能摘要）、Hacker News 的 Show HN 和 Paul Graham（抓取失败且无法摘要）。</li>' +
      '<li>点英文文章的「摘要」：按钮放在顶部工具栏（A）、底部浮动条（B）、标题下方（C），哪个手最顺？</li>' +
      '<li>「未读 / 全部」：ActivityBar 图标（A）、侧栏分段（B）、列表头标签（C）。</li>' +
      '<li>点「模拟后台同步」，再看新文章提示；点阮一峰周刊里的图片试图片查看器。</li>' +
      '</ul>' +
      '<div class="q">选择会记在地址栏 #A / #B / #C 和本地存储里，刷新后不变。</div>' +
      '</div>';
  }

  function ctxMenu() {
    var m = S.menu;
    if (!m) return '';
    var h = '';
    if (m.kind === 'article') {
      var a = artById(m.id);
      h += '<button data-act="m-read" data-id="' + a.id + '">' + icon(a.read ? 'mail' : 'mailOpen') + (a.read ? '标为未读' : '标为已读') + '</button>';
      h += '<div class="sep"></div>';
      h += '<button data-act="m-above" data-id="' + a.id + '">' + icon('up') + '此篇及以上标为已读<span class="hint">' + (S.snapshot.indexOf(a.id) + 1) + ' 篇</span></button>';
      h += '<button data-act="m-below" data-id="' + a.id + '">' + icon('down') + '此篇及以下标为已读<span class="hint">' + (S.snapshot.length - S.snapshot.indexOf(a.id)) + ' 篇</span></button>';
      h += '<div class="sep"></div>';
      h += '<button data-act="browser" data-id="' + a.id + '">' + icon('ext') + '在浏览器打开</button>';
    } else {
      var sc = { type: m.type, id: m.id || null };
      var n = unreadIn(sc);
      h += '<div class="cap">' + esc(scopeName(sc)) + (sc.type === 'cat' ? '（含其下所有源）' : '') + '</div>';
      h += '<button data-act="m-scope" data-type="' + sc.type + '" data-id="' + (sc.id || '') + '"' + (n ? '' : ' disabled style="opacity:.5"') + '>' + icon('checks') + '全部标为已读<span class="hint">' + n + ' 篇</span></button>';
    }
    return '<div class="ctx" style="left:' + m.x + 'px;top:' + m.y + 'px">' + h + '</div>';
  }
  function viewerHtml() {
    var v = S.viewer;
    if (!v) return '';
    var many = v.images.length > 1;
    return '<div class="viewer" data-act="v-close">' +
      '<div class="v-top"><span>' + (v.idx + 1) + ' / ' + v.images.length + '</span><span class="grow"></span><button class="v-btn" data-act="v-close" title="关闭（Esc）">' + icon('x') + '</button></div>' +
      (many ? '<button class="v-btn v-nav v-prev" data-act="v-prev">' + icon('left') + '</button><button class="v-btn v-nav v-next" data-act="v-next">' + icon('right') + '</button>' : '') +
      '<img src="' + v.images[v.idx] + '" style="transform:scale(' + v.zoom + ')" data-act="v-noop" alt="">' +
      '<div class="v-bottom"><button class="v-btn" data-act="v-zout" title="缩小">' + icon('zout') + '</button><span>' + Math.round(v.zoom * 100) + '%</span><button class="v-btn" data-act="v-zin" title="放大">' + icon('zin') + '</button></div>' +
      '</div>';
  }
  function settingsHtml() {
    if (!S.settings) return '';
    return '<div class="modal-mask" data-act="settings-close"><div class="modal" data-act="v-noop"><h2>设置</h2>' +
      '<div style="color:var(--text-3);font-size:13px">（设置面板不在本原型范围内；这里只示意入口位置。按 mrrss-280.4 裁决保留的分组：）</div>' +
      '<ul><li>FreshRSS 连接</li><li>百度翻译凭据（标题翻译）</li><li>大模型：端点 / 密钥 / 模型 + 测试连接（摘要）</li><li>网络与代理</li><li>托盘、关闭到托盘、开机自启</li><li>更新检查</li></ul>' +
      '<button class="btn" data-act="settings-close">关闭</button></div></div>';
  }
  function toastHtml() {
    if (!S.toast) return '';
    return '<div class="toast"><span>' + esc(S.toast.text) + '</span>' + (S.toast.undo ? '<button data-act="undo">撤销</button>' : '') + '</div>';
  }

  // ---------------- 渲染 ----------------
  var root = document.getElementById('root');
  var lastSelected = null;
  function render(resetDetail) {
    var ls = root.querySelector('.js-list'); var lsTop = ls ? ls.scrollTop : 0;
    var ds = root.querySelector('.js-detail'); var dsTop = ds ? ds.scrollTop : 0;
    var body = S.variant === 'A' ? variantA() : S.variant === 'B' ? variantB() : variantC();
    root.innerHTML = protoBar() + '<div class="app">' + body + '</div>' + notesPanel() + ctxMenu() + viewerHtml() + settingsHtml() + toastHtml();
    var nl = root.querySelector('.js-list'); if (nl) nl.scrollTop = lsTop;
    var nd = root.querySelector('.js-detail'); if (nd) nd.scrollTop = (resetDetail || lastSelected !== S.selected) ? 0 : dsTop;
    lastSelected = S.selected;
  }

  // ---------------- 事件 ----------------
  function closest(el, sel) { while (el && el !== root) { if (el.matches && el.matches(sel)) return el; el = el.parentNode; } return null; }

  root.addEventListener('click', function (e) {
    // 正文图片 → 图片查看器
    if (e.target.tagName === 'IMG' && closest(e.target, '.article-body')) {
      var imgs = Array.prototype.map.call(closest(e.target, '.article-body').querySelectorAll('img'), function (x) { return x.getAttribute('src'); });
      S.viewer = { images: imgs, idx: imgs.indexOf(e.target.getAttribute('src')), zoom: 1 };
      S.menu = null; render(); return;
    }
    var t = closest(e.target, '[data-act]');
    var hadMenu = !!S.menu;
    S.menu = null;
    if (!t) { if (hadMenu) render(); return; }
    var act = t.getAttribute('data-act'), id = t.getAttribute('data-id');
    if (t.disabled) return;
    switch (act) {
      case 'toggle-cat': e.stopPropagation(); S.collapsed[id] = !S.collapsed[id]; render(); break;
      case 'scope': setScope(t.getAttribute('data-type'), t.getAttribute('data-id') || null); break;
      case 'view': setView(t.getAttribute('data-v')); break;
      case 'open': openArticle(id); break;
      case 'summary': summarize(id); break;
      case 'browser': openInBrowser(id); break;
      case 'toggle-read': var a = artById(id); a.read = !a.read; render(); break;
      case 'refresh': simulateSync(true); break;
      case 'load-new': takeSnapshot(); render(); break;
      case 'settings': S.settings = true; render(); break;
      case 'settings-close': if (e.target === t || t.tagName === 'BUTTON') { S.settings = false; render(); } break;
      case 'm-read': var b = artById(id); b.read = !b.read; render(); break;
      case 'm-above': markAboveBelow(id, 'above'); break;
      case 'm-below': markAboveBelow(id, 'below'); break;
      case 'm-scope': markScope({ type: t.getAttribute('data-type'), id: t.getAttribute('data-id') || null }); break;
      case 'undo': if (S.toast && S.toast.undo) { var u = S.toast.undo; S.toast = null; u(); } break;
      case 'v-close': if (e.target === t || t.tagName === 'BUTTON') { S.viewer = null; render(); } break;
      case 'v-prev': S.viewer.idx = (S.viewer.idx - 1 + S.viewer.images.length) % S.viewer.images.length; S.viewer.zoom = 1; render(); break;
      case 'v-next': S.viewer.idx = (S.viewer.idx + 1) % S.viewer.images.length; S.viewer.zoom = 1; render(); break;
      case 'v-zin': S.viewer.zoom = Math.min(4, S.viewer.zoom + 0.25); render(); break;
      case 'v-zout': S.viewer.zoom = Math.max(0.5, S.viewer.zoom - 0.25); render(); break;
      case 'v-noop': if (hadMenu) render(); break;
      case 'variant':
        S.variant = t.getAttribute('data-v');
        try { localStorage.setItem('mrrss-proto-variant', S.variant); } catch (err) {}
        try { if (location.hash !== '#' + S.variant) location.hash = S.variant; } catch (err) {}
        render(); break;
      case 'notes':
        S.notes = !S.notes;
        try { localStorage.setItem('mrrss-proto-notes', S.notes ? '1' : '0'); } catch (err) {}
        render(); break;
      case 'sim-sync': simulateSync(false); break;
      case 'reset': resetAll(); break;
      default: if (hadMenu) render();
    }
  });

  root.addEventListener('contextmenu', function (e) {
    var t = closest(e.target, '[data-ctx]');
    if (!t) return;
    e.preventDefault();
    var kind = t.getAttribute('data-ctx');
    var x = Math.min(e.clientX, window.innerWidth - 240), y = Math.min(e.clientY, window.innerHeight - 190);
    if (kind === 'article') S.menu = { kind: 'article', id: t.getAttribute('data-id'), x: x, y: y };
    else S.menu = { kind: 'scope', type: t.getAttribute('data-type'), id: t.getAttribute('data-id') || null, x: x, y: y };
    render();
  });

  root.addEventListener('wheel', function (e) {
    if (!S.viewer || !closest(e.target, '.viewer')) return;
    e.preventDefault();
    S.viewer.zoom = Math.max(0.5, Math.min(4, S.viewer.zoom + (e.deltaY < 0 ? 0.15 : -0.15)));
    render();
  }, { passive: false });

  document.addEventListener('keydown', function (e) {
    if (e.key === 'Escape') { S.menu = null; S.viewer = null; S.settings = false; render(); }
    else if (S.viewer && e.key === 'ArrowLeft') { S.viewer.idx = (S.viewer.idx - 1 + S.viewer.images.length) % S.viewer.images.length; render(); }
    else if (S.viewer && e.key === 'ArrowRight') { S.viewer.idx = (S.viewer.idx + 1) % S.viewer.images.length; render(); }
  });
  window.addEventListener('hashchange', function () { S.variant = loadVariant(); render(); });
  window.addEventListener('resize', function () { if (S.menu) { S.menu = null; render(); } });

  takeSnapshot();
  render();
})();
