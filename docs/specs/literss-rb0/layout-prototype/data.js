/* MrRSS 布局原型：假数据（mrrss-280.5）。一次性原型代码，不进入正式实现。 */
(function () {
  'use strict';

  // 占位图：本地生成的 SVG，不依赖网络
  function img(label, c1, c2, w, h) {
    w = w || 1200; h = h || 675;
    var svg =
      '<svg xmlns="http://www.w3.org/2000/svg" width="' + w + '" height="' + h + '" viewBox="0 0 ' + w + ' ' + h + '">' +
      '<defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="' + c1 + '"/><stop offset="1" stop-color="' + c2 + '"/></linearGradient></defs>' +
      '<rect width="100%" height="100%" fill="url(#g)"/>' +
      '<g fill="rgba(255,255,255,.18)"><circle cx="' + w * 0.8 + '" cy="' + h * 0.25 + '" r="' + h * 0.3 + '"/><rect x="' + w * 0.08 + '" y="' + h * 0.6 + '" width="' + w * 0.5 + '" height="' + h * 0.12 + '" rx="12"/></g>' +
      '<text x="50%" y="48%" text-anchor="middle" font-family="sans-serif" font-size="' + Math.round(h / 11) + '" fill="#fff">' + label + '</text>' +
      '</svg>';
    return 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(svg);
  }

  var categories = [
    { id: 'c-news', name: '科技资讯' },
    { id: 'c-en', name: '英文技术' },
    { id: 'c-blog', name: '个人博客' }
  ];

  var feeds = [
    { id: 'f-36kr', cat: 'c-news', name: '36氪', color: '#2b6cf6', site: 'https://36kr.com' },
    { id: 'f-ithome', cat: 'c-news', name: 'IT之家', color: '#d83b2a', site: 'https://www.ithome.com' },
    { id: 'f-sspai', cat: 'c-news', name: '少数派', color: '#c9302c', site: 'https://sspai.com' },
    { id: 'f-ars', cat: 'c-en', name: 'Ars Technica', color: '#ff4e00', site: 'https://arstechnica.com' },
    { id: 'f-verge', cat: 'c-en', name: 'The Verge', color: '#5200ff', site: 'https://www.theverge.com' },
    { id: 'f-lwn', cat: 'c-en', name: 'LWN.net', color: '#555555', site: 'https://lwn.net' },
    { id: 'f-hn', cat: 'c-en', name: 'Hacker News', color: '#ff6600', site: 'https://news.ycombinator.com' },
    { id: 'f-ruanyf', cat: 'c-blog', name: '阮一峰的网络日志', color: '#2f7d4f', site: 'https://www.ruanyifeng.com/blog' },
    { id: 'f-simon', cat: 'c-blog', name: "Simon Willison's Weblog", color: '#1f6f8b', site: 'https://simonwillison.net' },
    { id: 'f-jvns', cat: 'c-blog', name: 'Julia Evans', color: '#b0357b', site: 'https://jvns.ca' },
    { id: 'f-pg', cat: null, name: 'Paul Graham: Essays', color: '#7a6a3a', site: 'http://paulgraham.com' }
  ];

  function p(list) { return list.map(function (t) { return '<p>' + t + '</p>'; }).join(''); }

  var articles = [];
  var nextId = 1;
  function add(a) {
    a.id = 'a' + nextId++;
    a.read = !!a.read;
    a.lang = a.lang || 'zh';
    a.kind = a.kind || 'full'; // full | trunc-ok | trunc-fail-long | trunc-fail-short
    a.images = a.images || [];
    a.url = a.url || (feedById(a.feed).site + '/p/' + a.id);
    articles.push(a);
    return a;
  }
  function feedById(id) { for (var i = 0; i < feeds.length; i++) if (feeds[i].id === id) return feeds[i]; }

  // ---- 36氪：量大、只需扫一眼的快讯 ----
  var krTitles = [
    '9 点 1 氪｜某头部新能源车企 9 月交付量同比增长 41%',
    '半导体设备厂商完成 12 亿元 C 轮融资，国资领投',
    '某连锁咖啡品牌宣布三季度新开门店 1200 家',
    '36氪首发｜具身智能公司获 5 亿元 A+ 轮融资',
    '国家统计局：8 月规模以上工业增加值同比增长 5.1%',
    '某电商平台调整商家佣金规则，10 月 15 日起执行',
    '最前线｜两家头部外卖平台在县域市场开启补贴战',
    '港股收盘：恒生指数涨 1.2%，科技股普涨',
    '智能眼镜出货量二季度环比翻倍，价格带下探至千元',
    '某储能企业拟赴港上市，已递交招股书',
    '8 点 1 氪｜多家银行下调存量房贷利率',
    '某自动驾驶公司宣布在 3 座城市开展无人出租车收费运营',
    '氪星晚报｜多家手机厂商发布秋季新品',
    '某大模型公司发布新一代推理模型，价格下调 60%',
    '光伏组件价格连续第三周小幅回升',
    '某奶茶品牌门店数突破 2 万家，加盟商回本周期拉长',
    '跨境电商平台三季度 GMV 同比增长 28%',
    '某机器人公司完成数亿元 B 轮融资，产品已进厂',
    '创投日报｜本周国内一级市场共披露 96 起融资事件',
    '某家电企业发布上半年财报，净利润同比增长 9%',
    '民航局：国庆假期预计日均旅客运输量 230 万人次',
    '某芯片设计公司科创板 IPO 获受理',
    '直击｜一场 AI 硬件发布会上的三个细节',
    '某云厂商宣布下调 GPU 算力租赁价格',
    '某新茶饮品牌启动海外扩张，首站东南亚',
    '上市公司公告速览：多家公司发布回购计划',
    '某二手车平台调整检测费用标准',
    '某 SaaS 公司宣布裁员 15%，聚焦核心产品',
    '深圳发布低空经济专项扶持政策',
    '某游戏公司新作上线首周流水破 3 亿元',
    '某物流企业试点无人配送车常态化运营',
    '9 点 1 氪｜某科技巨头推出新款折叠屏手机'
  ];
  var krBody = [
    '据公司公告，该事项已经董事会审议通过，后续将按规定履行信息披露义务。',
    '接近公司的人士透露，本轮资金将主要用于研发投入与产能扩张。',
    '业内人士认为，这一变化反映了行业在需求端的阶段性回暖，但持续性仍待观察。'
  ];
  krTitles.forEach(function (t, i) {
    add({
      feed: 'f-36kr', title: t, minutes: 6 + i * 23, read: i % 7 === 5,
      body: p([krBody[i % 3], krBody[(i + 1) % 3]]),
      snippet: krBody[i % 3]
    });
  });

  // ---- IT之家 ----
  [
    ['微软 Windows 11 25H2 正式推送，新增开始菜单分类视图', 18],
    ['苹果 iOS 27.1 测试版发布：修复蓝牙断连问题', 55],
    ['英伟达发布新一代笔记本显卡驱动，支持多帧生成', 140],
    ['小米澎湃 OS 3 开发版升级名单公布', 260],
    ['国产 PCIe 5.0 固态硬盘实测：顺序读取 14GB/s', 420]
  ].forEach(function (x, i) {
    add({
      feed: 'f-ithome', title: x[0], minutes: x[1], read: i === 4,
      body: p([
        'IT之家消息，' + x[0].split('，')[0] + '。据官方介绍，本次更新主要面向普通用户开放，此前已在测试渠道运行数周。',
        '从更新日志来看，除上述主要变化外，还包含若干稳定性修复与性能优化。用户可以前往系统设置检查更新。',
        'IT之家提醒，升级前请注意备份重要数据。'
      ]),
      images: i === 2 ? [img('驱动更新截图', '#1b5e20', '#76b900')] : [],
      snippet: '据官方介绍，本次更新主要面向普通用户开放，此前已在测试渠道运行数周。'
    });
  });

  // ---- 少数派：截断正文，点开自动抓全文成功 ----
  add({
    feed: 'f-sspai', title: '我的 2026 年桌面工作流：从收集到归档只用三个工具', minutes: 35,
    kind: 'trunc-ok',
    body: p(['工作流这件事，每年都会重写一次。今年我把工具砍到了三个：一个 RSS 阅读器，一个笔记软件，一个浏览器……']) +
      '<p class="rss-more">[阅读全文]</p>',
    full: p([
      '工作流这件事，每年都会重写一次。今年我把工具砍到了三个：一个 RSS 阅读器，一个笔记软件，一个浏览器。',
      '之所以砍，是因为我发现真正决定效率的不是工具有多强，而是每一步之间的摩擦有多小。每多一个「稍后读」列表，就多一个需要清理的地方。'
    ]) +
      '<h2>一、收集：只看未读</h2>' +
      p([
        '所有信息源都进 RSS，阅读器只开「未读」视图。扫标题，值得读的点开，不值得的整段标为已读。',
        '这里最关键的是「以上已读」这个动作：扫完一屏，右键最后一篇，把上面的全部清掉。'
      ]) +
      '<figure><img src="' + img('三栏阅读器截图', '#34495e', '#16a085') + '" alt="阅读器"><figcaption>图 1：只保留未读视图的阅读器</figcaption></figure>' +
      '<h2>二、细读：交给浏览器</h2>' +
      p([
        '需要细读或者剪藏的文章，直接在浏览器里打开。浏览器里有翻译插件、剪藏插件，也有我习惯的字体设置，没必要在阅读器里再做一遍。',
        '这样阅读器就只负责一件事：帮我决定哪些值得读。'
      ]) +
      '<figure><img src="' + img('浏览器剪藏', '#8e44ad', '#3498db') + '" alt="剪藏"><figcaption>图 2：在浏览器里剪藏到笔记</figcaption></figure>' +
      '<h2>三、归档：笔记软件</h2>' +
      p(['剪藏进来的内容统一打标签，周末花半小时整理。这部分和阅读器无关，就不展开了。']),
    images: [img('三栏阅读器截图', '#34495e', '#16a085'), img('浏览器剪藏', '#8e44ad', '#3498db')],
    snippet: '工作流这件事，每年都会重写一次。今年我把工具砍到了三个……'
  });
  add({
    feed: 'f-sspai', title: '派早报：多款新机发布、某笔记应用上线离线 AI 功能', minutes: 190, read: true,
    body: p(['早上好，今天是 9 月 27 日。以下是今天的派早报。', '某笔记应用今天推送更新，新增本地运行的 AI 整理功能，无需联网即可使用。', '另外，多家厂商在昨晚发布了秋季新品，我们挑了几款值得关注的整理在下面。'])
  });

  // ---- Ars Technica：英文，截断，自动抓全文成功 ----
  add({
    feed: 'f-ars', lang: 'en', title: 'NASA 推迟月球门户空间站第二阶段的发射计划',
    origTitle: 'NASA delays launch of the second phase of Lunar Gateway',
    minutes: 48, kind: 'trunc-ok',
    body: p(['NASA said on Thursday that the second phase of its Lunar Gateway program will slip by at least a year, citing budget pressure and…']) + '<p class="rss-more">Read the full story</p>',
    full: p([
      'NASA said on Thursday that the second phase of its Lunar Gateway program will slip by at least a year, citing budget pressure and integration delays with commercial partners.',
      'The Gateway, a small space station intended to orbit the Moon, was designed as a staging point for crewed landings. Its first two modules were originally scheduled to launch together.',
      'Officials said the agency is now reviewing whether the station remains necessary for early Artemis missions, a question critics have raised for years.'
    ]) +
      '<figure><img src="' + img('Lunar Gateway render', '#0b1d3a', '#3a6ea5') + '" alt="Gateway"><figcaption>An artist\'s rendering of the Gateway. (NASA)</figcaption></figure>' +
      p([
        '"We want to make sure every dollar spent on Gateway buys down risk for landing," an agency spokesperson said.',
        'Industry analysts expect the delay to shift some contract milestones into the next fiscal year.'
      ]),
    images: [img('Lunar Gateway render', '#0b1d3a', '#3a6ea5')],
    summary: [
      'NASA 宣布月球门户空间站第二阶段至少推迟一年，原因是预算压力和与商业伙伴的集成延误。',
      'NASA 正在重新评估早期阿尔忒弥斯任务是否仍需要这座空间站，这是批评者多年来的质疑。',
      '分析人士预计，部分合同里程碑会顺延到下一财年。'
    ]
  });

  // ---- The Verge：英文，截断，抓取失败，但 RSS 摘要够长 ----
  add({
    feed: 'f-verge', lang: 'en', title: '微软为 Windows 11 文件资源管理器加入标签页分组',
    origTitle: 'Microsoft is adding tab groups to File Explorer in Windows 11',
    minutes: 72, kind: 'trunc-fail-long', failReason: '全文抓取失败：站点返回 403 Forbidden（可能需要登录或拦截了程序访问）',
    body: p([
      'Microsoft is testing tab groups in File Explorer for Windows 11, bringing a feature from Edge to the file manager. Insiders in the Dev Channel can now drag one tab onto another to create a group, give it a name and a color, and collapse it to save space.',
      'The company says the feature is meant for people who keep many folders open at once, such as developers juggling several projects. Groups persist across restarts, and you can reopen a closed group from the new "Recent groups" menu.',
      'Microsoft has been steadily adding browser-like features to File Explorer since it introduced tabs in 2022. The update also includes a redesigned details pane and faster search indexing for network drives…'
    ]),
    summary: [
      '微软在 Windows 11 开发版中测试文件资源管理器的标签页分组，可命名、着色、折叠，重启后保留。',
      '面向同时打开大量文件夹的用户，例如同时处理多个项目的开发者。',
      '同批更新还有新的详情窗格和更快的网络驱动器搜索索引。'
    ]
  });

  // ---- LWN：英文，全文 ----
  add({
    feed: 'f-lwn', lang: 'en', title: '6.18 内核合并窗口第一周回顾',
    origTitle: 'The first half of the 6.18 merge window',
    minutes: 130,
    body: p([
      'As of this writing, 7,812 non-merge changesets have been pulled into the mainline repository for the 6.18 release. That is a bit more than half of what can be expected this time around.',
      'Some of the more significant changes merged so far include:'
    ]) + '<ul><li>The new "sheaves" slab allocator layer, which improves per-CPU caching.</li><li>Support for the "large block size" feature in the ext4 filesystem.</li><li>A number of Rust abstractions for the driver core.</li></ul>' +
      p(['The merge window can be expected to close on October 12, after which the stabilization process begins.']),
    summary: [
      '6.18 合并窗口第一周已并入 7812 个非合并变更，约为本轮预期的一半。',
      '重要变化：新的 sheaves slab 分配层、ext4 大块尺寸支持、若干驱动核心的 Rust 抽象。',
      '合并窗口预计 10 月 12 日关闭。'
    ]
  });

  // ---- Hacker News：英文，RSS 只有链接，抓取失败且太短 ----
  add({
    feed: 'f-hn', lang: 'en', title: 'Show HN：用 SQLite 实现的持久化消息队列',
    origTitle: 'Show HN: A durable message queue built on SQLite',
    minutes: 25, kind: 'trunc-fail-short', failReason: '全文抓取失败：连接超时（15 秒）',
    body: '<p><a>Comments</a></p>'
  });
  add({
    feed: 'f-hn', lang: 'en', title: '为什么我们从 Kubernetes 回到了单台服务器',
    origTitle: 'Why we moved from Kubernetes back to a single server',
    minutes: 95, kind: 'trunc-ok',
    body: '<p><a>Comments</a></p>',
    full: p([
      'Two years ago we ran eleven microservices on a managed Kubernetes cluster. Today everything runs on one large server with a hot standby.',
      'The short version: our traffic never needed horizontal scaling, and the operational overhead of the cluster was eating a full engineer\'s time.',
      'We kept the container images, so moving back would be straightforward if we ever needed to.'
    ]),
    summary: [
      '团队两年前在托管 Kubernetes 上跑 11 个微服务，现在全部迁到一台大服务器加热备。',
      '原因：流量从未需要横向扩展，集群运维占掉了一名工程师的全部时间。',
      '保留了容器镜像，将来需要时可以迁回。'
    ]
  });

  // ---- 阮一峰：中文，全文，多图 ----
  add({
    feed: 'f-ruanyf', title: '科技爱好者周刊（第 368 期）：信息过载的解药是减法', minutes: 300,
    body: p(['这里记录每周值得分享的科技内容，周五发布。']) +
      '<figure><img src="' + img('封面图：秋天的山谷', '#c0392b', '#f39c12') + '" alt="封面"><figcaption>（题图：秋天的山谷）</figcaption></figure>' +
      '<h2>封面图</h2>' + p(['四川稻城的秋天，山谷里的树叶全部变黄，是一年里最适合徒步的季节。']) +
      '<h2>本周话题：信息过载的解药是减法</h2>' +
      p([
        '很多人订阅了几百个信息源，结果每天打开阅读器，看到上千条未读，干脆就不看了。',
        '我的做法是减法：只订阅真正会读的源；对量大的源，只扫标题；扫完就整段标为已读，绝不留着「以后再看」。',
        '「以后再看」的意思往往就是「永远不看」。与其堆在一个列表里制造焦虑，不如承认它不重要。',
        '阅读器的功能也是一样。按钮越多，每一次决定越慢。一个好的阅读器应该让你用最少的动作，完成「这篇要不要读」的判断。'
      ]) +
      '<figure><img src="' + img('未读数对比', '#2c3e50', '#95a5a6') + '" alt="图表"><figcaption>减少订阅源前后，每日未读数的变化</figcaption></figure>' +
      '<h2>科技动态</h2>' +
      p([
        '1、某公司推出了一款只有一个按钮的相机。按下去就拍照，没有任何设置。',
        '2、研究发现，把手机调成黑白显示，每日使用时间平均下降 20 分钟。'
      ]) +
      '<figure><img src="' + img('单按钮相机', '#16a085', '#2ecc71') + '" alt="相机"></figure>' +
      '<h2>文章</h2>' + p(['1、如何写好一份技术文档（英文）：作者认为，文档最重要的是先回答读者的第一个问题。']),
    images: [img('封面图：秋天的山谷', '#c0392b', '#f39c12'), img('未读数对比', '#2c3e50', '#95a5a6'), img('单按钮相机', '#16a085', '#2ecc71')],
    snippet: '这里记录每周值得分享的科技内容，周五发布。'
  });

  // ---- Simon Willison：英文，全文 ----
  add({
    feed: 'f-simon', lang: 'en', title: '我现在如何用大语言模型写测试',
    origTitle: 'How I use LLMs to write tests now',
    minutes: 210,
    body: p([
      'I\'ve settled into a pattern for writing tests with LLM assistance that I think is worth describing.',
      'The key insight: I never ask the model to write tests for code it can\'t see. I paste the implementation, describe the edge cases I care about, and ask for tests that would fail if those edge cases broke.',
      'Then I run the tests, and — this is important — I deliberately break the code to confirm each test actually catches the failure it claims to catch.',
      'This takes a few extra minutes, but it has caught several tests that passed for the wrong reason.'
    ]),
    summary: [
      '作者用大模型写测试的固定做法：始终把实现代码贴给模型，并说明关心的边界情况。',
      '关键一步是故意改坏代码，确认每个测试真的能捕获它声称捕获的失败。',
      '多花几分钟，但已经抓到几个「因为错误原因而通过」的测试。'
    ]
  });

  // ---- Julia Evans：英文，全文，带图 ----
  add({
    feed: 'f-jvns', lang: 'en', title: '终端里的颜色到底是怎么工作的', origTitle: 'How terminal colours actually work',
    minutes: 480, read: false,
    body: p([
      'Hello! I\'ve been confused about terminal colours for years, so I finally sat down and figured out how they work.',
      'There are basically three kinds of colours your terminal program might use: the 16 "ANSI" colours, the 256-colour palette, and 24-bit "true colour".'
    ]) + '<figure><img src="' + img('ANSI 16 colours', '#222222', '#e74c3c') + '" alt="colours"></figure>' +
      p(['The 16 ANSI colours are the interesting ones, because your terminal emulator decides what they actually look like. That\'s why a program can look great in one terminal and unreadable in another.']),
    images: [img('ANSI 16 colours', '#222222', '#e74c3c')],
    summary: [
      '终端颜色分三类：16 色 ANSI、256 色调色板、24 位真彩色。',
      '16 色 ANSI 的实际颜色由终端模拟器决定，所以同一程序在不同终端里观感差别很大。'
    ]
  });

  // ---- Paul Graham：英文，截断，抓取失败且 RSS 太短 ----
  add({
    feed: 'f-pg', lang: 'en', title: '优秀的人如何选择要做的事', origTitle: 'How People Choose What to Work On',
    minutes: 1500, kind: 'trunc-fail-short', failReason: '全文抓取失败：页面结构无法识别正文',
    body: p(['September 2026'])
  });

  // 已读的旧文章，让「全部」视图与「未读」视图有差别
  [
    ['f-ithome', '国产操作系统市场份额突破 5%', 2000],
    ['f-36kr', '氪星晚报｜多家新能源车企公布 8 月销量', 2100],
    ['f-ruanyf', '科技爱好者周刊（第 367 期）：慢下来的技术', 10000],
    ['f-lwn', 'Rust 驱动进入主线的第二年', 9000, 'en', 'Rust drivers, two years in']
  ].forEach(function (x) {
    add({ feed: x[0], title: x[1], minutes: x[2], read: true, lang: x[3] || 'zh', origTitle: x[4], body: p(['（已读的旧文章，用来区分「未读」和「全部」视图。）']) });
  });

  articles.sort(function (a, b) { return a.minutes - b.minutes; });

  window.PROTO_DATA = { categories: categories, feeds: feeds, articles: articles, img: img };
})();
