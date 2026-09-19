// ===== 工具 =====
const $ = s => document.querySelector(s);
const api = p => fetch(p).then(r => r.json());
const email = () => $('#emailSel').value;
const qs = o => new URLSearchParams(Object.entries(o).filter(([k,v]) => v !== '' && v != null)).toString();

const fmt = {
  money: v => v == null ? '—' : '$' + Number(v).toFixed(v >= 1 ? 4 : 6),
  int:   v => v == null ? '—' : Number(v).toLocaleString(),
  pct:   v => v == null ? '—' : (v > 0 ? '+' : '') + Number(v).toFixed(1) + '%',
  ms:    v => v == null ? '—' : Math.round(v).toLocaleString() + ' ms',
  time:  s => s ? s.replace('T', ' ').slice(0, 19) : '—',
};
const cls = v => v == null ? '' : (v > 0 ? 'pos' : v < 0 ? 'neg' : '');
const fmtDayLocal = d => d.getFullYear() + '-' + String(d.getMonth()+1).padStart(2,'0') + '-' + String(d.getDate()).padStart(2,'0');

// ===== SVG 折线图 =====
function lineChart(el, series, opt = {}) {
  const color = opt.color || '#5b8cff';
  const fmtY = opt.fmtY || fmt.money;
  const H = opt.height || 240, W = 1100;
  el.innerHTML = '';
  if (!series || !series.length) {
    el.innerHTML = '<div class="dim" style="padding:40px;text-align:center">无数据</div>'; return;
  }
  const P = { l: 56, r: 14, t: 16, b: 34 };
  const iw = W - P.l - P.r, ih = H - P.t - P.b;
  const vs = series.map(d => d.v || 0);
  let min = Math.min(...vs, 0), max = Math.max(...vs, 0);
  if (max === min) max = min + 1;
  const x = i => P.l + (series.length === 1 ? iw / 2 : i / (series.length - 1) * iw);
  const y = v => P.t + ih - (v - min) / (max - min) * ih;
  const ns = 'http://www.w3.org/2000/svg';
  const svg = document.createElementNS(ns, 'svg');
  svg.setAttribute('viewBox', '0 0 ' + W + ' ' + H);
  const mk = (t, a) => { const e = document.createElementNS(ns, t); for (const k in a) e.setAttribute(k, a[k]); return e; };
  for (let i = 0; i <= 4; i++) {
    const v = min + (max - min) * i / 4, yy = y(v);
    svg.appendChild(mk('line', { x1: P.l, y1: yy, x2: W - P.r, y2: yy, stroke: '#2c3a56', 'stroke-width': Math.abs(v) < 1e-9 ? 1.4 : 0.6 }));
    const t = mk('text', { x: P.l - 8, y: yy + 4, 'text-anchor': 'end', fill: '#8b98b0', 'font-size': 11 });
    t.textContent = fmtY(v).replace('$', ''); svg.appendChild(t);
  }
  let d = 'M' + x(0) + ',' + y(vs[0]);
  for (let i = 1; i < series.length; i++) d += ' L' + x(i) + ',' + y(vs[i]);
  svg.appendChild(mk('path', { d: d + ' L' + x(series.length - 1) + ',' + y(0) + ' L' + x(0) + ',' + y(0) + ' Z', fill: color, opacity: 0.14 }));
  svg.appendChild(mk('path', { d, fill: 'none', stroke: color, 'stroke-width': 2, 'stroke-linejoin': 'round' }));
  if (series.length <= 60) vs.forEach((v, i) => svg.appendChild(mk('circle', { cx: x(i), cy: y(v), r: 2.4, fill: color })));
  const step = Math.max(1, Math.ceil(series.length / 8));
  series.forEach((s, i) => {
    if (i % step) return;
    const t = mk('text', { x: x(i), y: H - 10, 'text-anchor': 'middle', fill: '#8b98b0', 'font-size': 10 });
    t.textContent = s.k.length > 10 ? s.k.slice(5) : s.k; svg.appendChild(t);
  });
  const tip = mk('text', { x: P.l, y: P.t - 3, fill: '#e6ecf5', 'font-size': 12 });
  svg.appendChild(tip);
  svg.addEventListener('mousemove', ev => {
    const r = svg.getBoundingClientRect();
    const px = (ev.clientX - r.left) / r.width * W;
    let i = Math.round((px - P.l) / iw * (series.length - 1));
    i = Math.max(0, Math.min(series.length - 1, i));
    tip.textContent = series[i].k + '   ' + fmtY(series[i].v);
    tip.setAttribute('x', Math.min(W - 170, Math.max(P.l, x(i) - 40)));
  });
  el.appendChild(svg);
}

// ===== 横向条形 =====
function hbar(el, items, fmtV) {
  fmtV = fmtV || fmt.money;
  el.innerHTML = '';
  if (!items || !items.length) { el.innerHTML = '<div class="dim" style="padding:20px">无数据</div>'; return; }
  const max = Math.max(...items.map(i => i.v || 0), 1e-9);
  items.forEach(it => {
    const row = document.createElement('div'); row.className = 'bar-row';
    const k = document.createElement('div'); k.className = 'k'; k.textContent = it.k; k.title = it.k;
    const track = document.createElement('div'); track.className = 'track';
    const fill = document.createElement('div'); fill.className = 'fill';
    fill.style.width = Math.max(1, (it.v || 0) / max * 100) + '%';
    track.appendChild(fill);
    const v = document.createElement('div'); v.className = 'v';
    v.textContent = fmtV(it.v) + (it.n != null ? '  · ' + fmt.int(it.n) + '笔' : '');
    row.append(k, track, v); el.appendChild(row);
  });
}

function cards(el, defs) {
  el.innerHTML = '';
  defs.forEach(d => {
    const c = document.createElement('div'); c.className = 'card';
    const l = document.createElement('div'); l.className = 'label'; l.textContent = d.label;
    const v = document.createElement('div'); v.className = 'value ' + (d.cls || ''); v.textContent = d.value;
    c.append(l, v);
    if (d.sub) { const s = document.createElement('div'); s.className = 'sub'; s.textContent = d.sub; c.appendChild(s); }
    el.appendChild(c);
  });
}

const td = t => { const e = document.createElement('td'); e.innerHTML = t; return e; };
const tn = (v, f, c) => { const e = document.createElement('td'); e.className = 'num ' + (c || ''); e.textContent = (f || (x => x))(v); return e; };

// ===== Tab =====
document.querySelectorAll('.tab').forEach(b => b.addEventListener('click', () => {
  document.querySelectorAll('.tab').forEach(x => x.classList.toggle('active', x === b));
  document.querySelectorAll('.page').forEach(p => p.classList.toggle('active', p.id === 'tab-' + b.dataset.tab));
  load(b.dataset.tab);
}));
$('#emailSel').addEventListener('change', () => load(document.querySelector('.tab.active').dataset.tab));
async function load(t) {
  if (t === 'overview') return loadOverview();
  if (t === 'income')   return loadIncome();
  if (t === 'usage')    return loadUsage();
  if (t === 'ledger')   return loadLedger();
  if (t === 'sync')     return loadSync();
}

// ===== 总览 =====
async function loadOverview() {
  const d = await api('/api/overview');
  $('#now').textContent = '更新于 ' + fmt.time(d.now) + ' · 当前小时 ' + d.cur_hour +
    (d.elapsed_minutes != null ? ' · 已过 ' + d.elapsed_minutes + ' 分钟(同环比按此前N分钟口径)' : '');
  const r = d.rows || [];
  const sum = f => r.reduce((a, x) => a + (x[f] || 0), 0);
  cards($('#overviewCards'), [
    { label: '账号数', value: r.length, sub: '已登录会话' },
    { label: '7D 总请求', value: fmt.int(d.total_usage_7d), sub: '全部账号' },
    { label: '分账收入合计', value: fmt.money(sum('share_income')), sub: 'account_share_income' },
    { label: '当前小时收入', value: fmt.money(sum('cur_hour_income')), sub: d.cur_hour },
    { label: '今日收入', value: fmt.money(sum('today_income')), sub: '昨日 ' + fmt.money(sum('yesterday_income')) },
    { label: '今日时均收入', value: fmt.money(sum('today_hour_avg')), sub: '今日收入/已过去小时' },
  ]);
  const tb = $('#overviewTable tbody'); tb.innerHTML = '';
  r.forEach(x => {
    const tr = document.createElement('tr');
    tr.append(td(x.email),
      tn(x.usage_7d, fmt.int), tn(x.usage_pct_7d, fmt.pct),
      tn(x.total_cost), tn(x.actual_cost), tn(x.share_income),
      tn(x.cur_hour_income), tn(x.prev_hour_income),
      tn(x.hour_qoq, fmt.pct, cls(x.hour_qoq)),
      tn(x.hour_yoy, fmt.pct, cls(x.hour_yoy)),
      tn(x.today_income), tn(x.today_hour_avg),
      tn(x.day_qoq, fmt.pct, cls(x.day_qoq)),
      tn(x.balance));
    tb.appendChild(tr);
  });
  $('#notes').innerHTML =
    '<h3>首页指标口径</h3><ul>' +
    '<li><b>7D请求/占比</b>：近 7 个自然日 usage_logs 请求数及占全部账号比例，衡量各账号近期活跃度。</li>' +
    '<li><b>账户计费</b>：total_cost，官方按定价计算的原始费用。</li>' +
    '<li><b>用户扣费</b>：actual_cost，乘 rate_multiplier 后实际从用户扣的金额（平台营收基准）。</li>' +
    '<li><b>分账收入</b>：balance_ledger 中 direction=credit 且 reason=account_share_income 的入账，即号主分账收入。</li>' +
    '<li><b>当前/上一小时收入</b>：分账收入按小时聚合。环比=当前小时/上一小时−1；同比=当前小时/昨天同一小时−1。</li>' +
    '<li><b>今日时均</b>：今日累计分账收入 ÷ 今日已过去小时数，用于预估全天收入水平。</li>' +
    '</ul><h3>推荐分析维度及理由</h3><ul>' +
    '<li><b>分账收入趋势（时/天）</b>：核心业务指标，识别高峰时段与增长/衰退拐点。</li>' +
    '<li><b>收入 Top 使用者/上游账户</b>：定位贡献收入的头部用户与被调用的上游账户，指导运营。</li>' +
    '<li><b>按模型用量</b>：不同模型成本差异大，拆模型看计费/实扣/token 结构。</li>' +
    '<li><b>毛利（收入−扣费）</b>：分账收入与用户扣费之差，反映分账后的净留存空间。</li>' +
    '<li><b>请求类型/耗时</b>：stream/sync 占比与平均耗时，评估负载与体验特征。</li>' +
    '</ul><h3>不分析的字段及原因</h3><ul>' +
    '<li><b>request_id / ref_id</b>：逐条唯一标识，聚合无统计意义，仅用于明细追溯与对账。</li>' +
    '<li><b>balance_after 逐点</b>：时点快照，余额趋势用按天采样已足够，逐条展示重复。</li>' +
    '<li><b>user_agent</b>：客户端 UA 分布，与计费/收入无直接关联，信息密度低。</li>' +
    '<li><b>first_token_ms</b>：流式首 token 延迟，非流式为空、样本缺失多，聚合易失真。</li>' +
    '<li><b>inbound_endpoint / group_id</b>：当前取值维度单一，区分度不足。</li>' +
    '</ul>';
}

// ===== 分账收入 =====
async function loadIncome() {
  const e = email();
  const ov = await api('/api/overview');
  const r = (ov.rows || []).filter(x => !e || x.email === e);
  const sum = f => r.reduce((a, x) => a + (x[f] || 0), 0);
  cards($('#incomeCards'), [
    { label: '分账收入合计', value: fmt.money(sum('share_income')) },
    { label: '当前小时', value: fmt.money(sum('cur_hour_income')), sub: '环比 ' + fmt.pct(r[0] && r[0].hour_qoq) + ' · 同比 ' + fmt.pct(r[0] && r[0].hour_yoy) },
    { label: '上一小时', value: fmt.money(sum('prev_hour_income')) },
    { label: '今日收入', value: fmt.money(sum('today_income')), sub: '日环比 ' + fmt.pct(r[0] && r[0].day_qoq) },
    { label: '今日时均', value: fmt.money(sum('today_hour_avg')) },
  ]);
  const hd = await api('/api/income/hourly?' + qs({ email: e, days: $('#incomeHourlyDays').value }));
  lineChart($('#incomeHourlyChart'), hd.series, { color: '#3ecf8e' });
  const dd = await api('/api/income/daily?' + qs({ email: e, days: $('#incomeDailyDays').value }));
  lineChart($('#incomeDailyChart'), dd.series, { color: '#5b8cff' });
  hbar($('#topConsumer'), (await api('/api/income/top?' + qs({ email: e, dim: 'consumer_user_id' }))).items);
  hbar($('#topAccount'),  (await api('/api/income/top?' + qs({ email: e, dim: 'account_id' }))).items);
}
$('#incomeHourlyDays').addEventListener('change', async () => {
  const d = await api('/api/income/hourly?' + qs({ email: email(), days: $('#incomeHourlyDays').value }));
  lineChart($('#incomeHourlyChart'), d.series, { color: '#3ecf8e' });
});
$('#incomeDailyDays').addEventListener('change', async () => {
  const d = await api('/api/income/daily?' + qs({ email: email(), days: $('#incomeDailyDays').value }));
  lineChart($('#incomeDailyChart'), d.series, { color: '#5b8cff' });
});

// ===== 用量分析 =====
async function loadUsage() {
  const e = email();
  const d = await api('/api/usage/summary?' + qs({ email: e }));
  const s = d.summary || {};
  cards($('#usageCards'), [
    { label: '总请求', value: fmt.int(s.requests) },
    { label: '输出 tokens', value: fmt.int(s.output_tokens), sub: '输入 ' + fmt.int(s.input_tokens) },
    { label: '缓存读 tokens', value: fmt.int(s.cache_read_tokens) },
    { label: '账户计费', value: fmt.money(s.total_cost) },
    { label: '用户扣费', value: fmt.money(s.actual_cost) },
    { label: '平均耗时', value: fmt.ms(s.avg_duration_ms) },
  ]);
  const bm = await api('/api/usage/by-model?' + qs({ email: e }));
  const tb = $('#byModelTable tbody'); tb.innerHTML = '';
  (bm.items || []).forEach(m => {
    const tr = document.createElement('tr');
    tr.append(td(m.model), tn(m.requests, fmt.int), tn(m.input_tokens, fmt.int),
      tn(m.output_tokens, fmt.int), tn(m.cache_read_tokens, fmt.int),
      tn(m.total_cost), tn(m.actual_cost), tn(m.avg_duration_ms, fmt.ms), tn(m.avg_rate_multiplier));
    tb.appendChild(tr);
  });
  const dist = d.dist || {};
  let dh = '';
  [['request_type', dist.request_type], ['billing_mode', dist.billing_mode], ['stream', dist.stream]].forEach(([t, arr]) => {
    if (!arr || !arr.length) return;
    const tot = arr.reduce((a, x) => a + x.v, 0) || 1;
    dh += '<h3 style="margin:12px 0 6px;color:var(--dim);font-size:12px">' + t + '</h3>';
    arr.forEach(x => {
      dh += '<div class="bar-row"><div class="k">' + x.k + '</div><div class="track"><div class="fill" style="width:' + (x.v / tot * 100) + '%"></div></div><div class="v">' + fmt.int(x.v) + ' · ' + (x.v / tot * 100).toFixed(1) + '%</div></div>';
    });
  });
  $('#reqDist').innerHTML = dh || '<div class="dim">无数据</div>';
  const dd = await api('/api/usage/daily?' + qs({ email: e, days: $('#usageDailyDays').value }));
  lineChart($('#usageDailyChart'), (dd.items || []).map(x => ({ k: x.day, v: x.requests })), { color: '#f5b942', fmtY: v => fmt.int(v) });
  const ut = $('#usageDailyTable tbody'); ut.innerHTML = '';
  (dd.items || []).slice().reverse().forEach(x => {
    const margin = (x.income || 0) - (x.actual_cost || 0);
    const tr = document.createElement('tr');
    tr.append(td(x.day), tn(x.requests, fmt.int), tn(x.output_tokens, fmt.int),
      tn(x.total_cost), tn(x.actual_cost), tn(x.income), tn(margin, fmt.money, cls(margin)));
    ut.appendChild(tr);
  });
  const day = $('#usageHourlyDay').value || fmtDayLocal(new Date());
  $('#usageHourlyDay').value = day;
  const hh = await api('/api/usage/hourly?' + qs({ email: e, day }));
  lineChart($('#usageHourlyChart'), hh.series.map(x => ({ k: x.k + ':00', v: x.v })), { color: '#b98cff', fmtY: v => fmt.int(v) });
}
$('#usageDailyDays').addEventListener('change', async () => {
  const d = await api('/api/usage/daily?' + qs({ email: email(), days: $('#usageDailyDays').value }));
  lineChart($('#usageDailyChart'), (d.items || []).map(x => ({ k: x.day, v: x.requests })), { color: '#f5b942', fmtY: v => fmt.int(v) });
});
$('#usageHourlyDay').addEventListener('change', async () => {
  const d = await api('/api/usage/hourly?' + qs({ email: email(), day: $('#usageHourlyDay').value }));
  lineChart($('#usageHourlyChart'), d.series.map(x => ({ k: x.k + ':00', v: x.v })), { color: '#b98cff', fmtY: v => fmt.int(v) });
});

// ===== 流水明细 =====
let pgOffset = 0; const pgLimit = 100;
async function loadLedger() {
  const p = { email: email(), direction: $('#fDir').value, reason: $('#fReason').value, limit: pgLimit, offset: pgOffset };
  const d = await api('/api/ledger?' + qs(p));
  if (!$('#fReason').dataset.init) {
    (d.reasons || []).forEach(r => { const o = document.createElement('option'); o.value = r.k; o.textContent = r.k + ' (' + fmt.int(r.v) + ')'; $('#fReason').appendChild(o); });
    $('#fReason').dataset.init = '1';
  }
  $('#ledgerTotal').textContent = '共 ' + fmt.int(d.total) + ' 条';
  const tb = $('#ledgerTable tbody'); tb.innerHTML = '';
  (d.items || []).forEach(l => {
    const m = l.metadata || {};
    const tr = document.createElement('tr');
    const dirTd = document.createElement('td');
    dirTd.innerHTML = '<span class="tag ' + l.direction + '">' + l.direction + '</span>';
    tr.append(td(fmt.time(l.created_at)), td(l.email), dirTd, td(l.reason),
      tn(l.amount, x => x, l.direction === 'credit' ? 'pos' : 'neg'),
      tn(l.balance_after, x => x, 'dim'),
      tn(m.consumer_user_id != null ? m.consumer_user_id : '—', x => x, 'dim'),
      tn(m.api_key_id != null ? m.api_key_id : '—', x => x, 'dim'),
      tn(m.account_id != null ? m.account_id : '—', x => x, 'dim'),
      td('<span class="dim" style="font-size:11px">' + (m.request_id || '—') + '</span>'));
    tb.appendChild(tr);
  });
  const pages = Math.max(1, Math.ceil((d.total || 0) / pgLimit));
  $('#pgInfo').textContent = Math.floor(pgOffset / pgLimit) + 1 + ' / ' + pages;
  $('#pgPrev').disabled = pgOffset <= 0;
  $('#pgNext').disabled = pgOffset + pgLimit >= (d.total || 0);
}
$('#fGo').addEventListener('click', () => { pgOffset = 0; loadLedger(); });
$('#pgPrev').addEventListener('click', () => { pgOffset = Math.max(0, pgOffset - pgLimit); loadLedger(); });
$('#pgNext').addEventListener('click', () => { pgOffset += pgLimit; loadLedger(); });

// ===== init =====
(async () => {
  const e = await api('/api/emails');
  (e.emails || []).forEach(x => { const o = document.createElement('option'); o.value = x; o.textContent = x; $('#emailSel').appendChild(o); });
  loadOverview();
})();


// ===== 数据同步 =====
let syncTimer = null;

async function postJSON(url, body) {
  const r = await fetch(url, { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify(body || {}) });
  return r.json();
}

async function loadSync() {
  const d = await api('/api/sync/status');
  const enabled = d.sync_enabled;
  $('#syncEnabled').textContent = enabled ? '' : '未启用';
  $('#syncDisabledTip').style.display = enabled ? 'none' : 'block';
  $('#btnInit').disabled = true;
  $('#btnUpdate').disabled = true;
  $('#btnBackfill').disabled = true;
  $('#chkAuto').disabled = true;
  if (!enabled) { $('#syncNotes').innerHTML = syncNotesHTML(); stopSyncPoll(); return; }
  const st = d.status || {};
  $('#chkAuto').checked = !!st.auto;
  $('#chkAuto').disabled = false;
  // 初始化按钮：仅当库中无数据
  $('#btnInit').disabled = st.has_data || st.running;
  $('#btnUpdate').disabled = !!st.running;
  $('#btnBackfill').disabled = !!st.running;
  // 覆盖水位
  renderWatermarks(st);
  // 当前任务/进度
  renderProgress(st);
  // 历史
  const jobs = await api('/api/sync/jobs?limit=20');
  renderJobs(jobs.jobs || []);
  $('#syncNotes').innerHTML = syncNotesHTML();
  // 若正在跑，开启轮询
  if (st.running) startSyncPoll(); else stopSyncPoll();
}

function startSyncPoll() {
  if (syncTimer) return;
  syncTimer = setInterval(async () => {
    if (document.querySelector('.tab.active').dataset.tab !== 'sync') { stopSyncPoll(); return; }
    const d = await api('/api/sync/status');
    const st = d.status || {};
    renderProgress(st);
    $('#btnInit').disabled = st.has_data || st.running;
    $('#btnUpdate').disabled = !!st.running;
    $('#btnBackfill').disabled = !!st.running;
    if (!st.running) { stopSyncPoll(); loadSync(); }
  }, 1500);
}
function stopSyncPoll(){ if (syncTimer){ clearInterval(syncTimer); syncTimer = null; } }

function renderWatermarks(st) {
  const wm = st.watermarks || {}, rg = st.ranges || {};
  const emails = Object.keys(rg);
  if (!emails.length) { $('#wmList').textContent = '无账号'; return; }
  let h = '';
  emails.forEach(e => {
    const u = (wm[e]||{}).usage, l = (wm[e]||{}).ledger;
    const ur = (rg[e]||{}).usage || {}, lr = (rg[e]||{}).ledger || {};
    const line = (label, r, w) =>
      label + ': ' + ((r.count||0) > 0
        ? fmt.int(r.count) + ' 条 · ' + fmt.time(r.min) + ' ~ ' + fmt.time(r.max)
        : '<span class="dim">无数据</span>') +
      ' · 水位 ' + (w ? fmt.time(w) : '<span class="dim">—</span>');
    h += '<div style="margin-bottom:10px"><b>' + e + '</b><br>' +
      line('usage', ur, u) + '<br>' + line('ledger', lr, l) + '</div>';
  });
  $('#wmList').innerHTML = h;
}

function renderProgress(st) {
  const p = st.current;
  const panel = $('#syncProgressPanel');
  // 没有正在跑的任务时，仍展示最近一次任务的进度快照
  const src = p || (st.latest_job && st.latest_job.progress_json ? JSON.parse(st.latest_job.progress_json) : null);
  if (!src) { panel.style.display = 'none'; return; }
  panel.style.display = 'block';
  const status = src.status || 'running';
  const jel = $('#jobStatus');
  jel.textContent = status;
  jel.className = 'tag ' + status;
  const pct = src.total_est_pages ? Math.min(100, src.fetched_pages / src.total_est_pages * 100) : 0;
  $('#progressFill').style.width = pct.toFixed(1) + '%';
  let txt = '块 ' + (src.done_chunks||0) + '/' + (src.total_chunks||0) +
    ' · 页 ' + (src.fetched_pages||0) + '/' + (src.total_est_pages||0) +
    ' · 条 ' + fmt.int(src.fetched_items||0) +
    (src.skipped ? ' · 跳过 ' + src.skipped + ' 块(已覆盖)' : '');
  if (status === 'running' && src.cur_kind) {
    txt += '<br>正在同步：' + (src.cur_email||'') + ' [' + kindLabel(src.cur_kind) + '] ' + (src.cur_range||'') +
      ' · 第 ' + (src.cur_page||0) + '/' + (src.cur_est_pages||'?') + ' 页';
  }
  if (status === 'running' && src.overall_eta) {
    txt += '<br>预计本项完成 ' + fmt.time(src.cur_chunk_eta) + ' · 预计整体完成 ' + fmt.time(src.overall_eta);
  }
  if (src.error) txt += '<br><span class="neg">错误: ' + src.error + '</span>';
  if (src.finished_at) txt += '<br>完成于 ' + fmt.time(src.finished_at);
  $('#progressText').innerHTML = txt;
  // chunk 表
  const latest = st.latest_job;
  if (latest && latest.plan_json) {
    try {
      const plan = JSON.parse(latest.plan_json);
      const tb = $('#chunkTable tbody'); tb.innerHTML = '';
      (plan.chunks || []).forEach(c => {
        const tr = document.createElement('tr');
        const st2 = c.done ? (c.err === 'skipped:covered' ? 'skipped' : 'done') :
          (status==='running' && src.cur_email===c.email && src.cur_kind===c.kind ? 'running' : 'pending');
        if (st2==='skipped') tr.className='chunk-skipped';
        const rng = c.kind==='snapshots' ? '当前快照' : fmt.time(c.start).slice(5) + ' ~ ' + fmt.time(c.end).slice(5);
        tr.append(td(c.email), td(kindLabel(c.kind)), td(rng),
          tn(c.est_pages, fmt.int), tn(c.pages||0, fmt.int), tn(c.items||0, fmt.int),
          td('<span class="tag '+st2+'">'+st2+'</span>'));
        tb.appendChild(tr);
      });
    } catch(e){}
  }
}

function kindLabel(k){ return {usage:'使用明细',ledger:'余额流水',snapshots:'汇总快照'}[k] || k; }

function renderJobs(jobs) {
  const tb = $('#jobTable tbody'); tb.innerHTML = '';
  if (!jobs.length) { tb.innerHTML = '<tr><td colspan="7" class="dim">暂无任务</td></tr>'; return; }
  jobs.forEach(j => {
    let items = '—';
    try { const p = JSON.parse(j.progress_json||'{}'); items = fmt.int(p.fetched_items||0); } catch(e){}
    const tr = document.createElement('tr');
    tr.append(td('#'+j.id), td(j.trigger_kind), td('<span class="tag '+j.status+'">'+j.status+'</span>'),
      td(j.started_at?fmt.time(j.started_at):'—'), td(j.finished_at?fmt.time(j.finished_at):'—'),
      td(items), td(j.error?'<span class="neg" style="font-size:11px">'+j.error.slice(0,60)+'</span>':'—'));
    tb.appendChild(tr);
  });
}

$('#btnInit').addEventListener('click', async () => {
  $('#btnInit').disabled = true;
  const r = await postJSON('/api/sync/init');
  if (r.reason) alert(r.reason);
  loadSync(); startSyncPoll();
});
$('#btnUpdate').addEventListener('click', async () => {
  $('#btnUpdate').disabled = true;
  await postJSON('/api/sync/update');
  loadSync(); startSyncPoll();
});
$('#btnBackfill').addEventListener('click', async () => {
  $('#btnBackfill').disabled = true;
  await postJSON('/api/sync/backfill');
  loadSync(); startSyncPoll();
});
$('#chkAuto').addEventListener('change', async () => {
  const r = await postJSON('/api/sync/auto', { on: $('#chkAuto').checked });
  $('#chkAuto').checked = !!r.auto;
});

function syncNotesHTML() {
  return '<ul>' +
    '<li><b>更新 vs 回填</b>："更新数据"只做增量（从覆盖点到前一分钟）；"回填最近30天"是单独大任务，适合首次拉历史或补大段缺失。</li>' +
    '<li><b>获取方案</b>：同步前按"账号 × 数据类型 × 时间块"生成方案。使用明细按天切块、流水按周切块、汇总快照整段一次。</li>' +
    '<li><b>跳过已覆盖</b>：水位记录每类数据已成功覆盖到的最大时间；完全落入水位的块直接跳过，不重复请求。</li>' +
    '<li><b>断点续传</b>：每块成功后才推进水位；中途停止下次从未覆盖处继续（按 id upsert，幂等不重复）。</li>' +
    '<li><b>更新截止</b>：手动/自动更新截止到按下时刻的前一分钟，避免拉到正在写入的当前分钟数据。</li>' +
    '<li><b>平滑限速</b>：单并发 + 固定约 1.2s/请求间隔；30 天初始化约几十~几百请求，对上游压力小。</li>' +
    '<li><b>安全</b>：同步 worker 是唯一持可写库和解密凭据的组件；全部分析接口仍走只读连接，机密不回显。</li>' +
    '</ul>';
}


// ===== 数据版本自动刷新 =====
// 同步任务一结算（done/failed），后端 data_version 即 +1。
// 前端每 3s 轻量轮询该版本，发现变化就静默重载当前页数据 —— 用户无需手动刷新/切tab。
let __lastDataVer = null;
let __dataVerTimer = null;

async function pollDataVersion() {
  try {
    const d = await api('/api/sync/status');
    if (!d.sync_enabled) return;              // 未启用同步：无需轮询
    const st = d.status || {};
    const v = st.data_version || 0;
    if (__lastDataVer === null) { __lastDataVer = v; return; } // 首次只记录基线
    if (v !== __lastDataVer) {
      __lastDataVer = v;
      // 静默刷新当前页
      const cur = document.querySelector('.tab.active');
      if (cur) {
        const t = cur.dataset.tab;
        // sync 页本身由 startSyncPoll 管；其余页直接重载
        if (t !== 'sync') load(t);
        else loadSync();
        flashNow('数据已更新 ' + new Date().toLocaleTimeString('zh-CN', {hour12:false}));
      }
    }
  } catch (e) { /* 网络抖动忽略，下轮再试 */ }
}

// 在右上角时间旁短暂提示"数据已更新"（3s 后恢复，不额外触发刷新）
function flashNow(text) {
  const el = $('#now');
  if (!el) return;
  const old = el.textContent;
  el.textContent = text;
  el.style.color = '#3ecf8e';
  setTimeout(() => { el.style.color = ''; el.textContent = old; }, 3000);
}

// 启动全局轮询（每3s，足够实时又不给本地服务压力）
function startDataVerWatch() {
  if (__dataVerTimer) return;
  __dataVerTimer = setInterval(pollDataVersion, 3000);
}
startDataVerWatch();


// ===== 关闭页面提醒（后台服务驻留提示）=====
// 场景：关闭/刷新/跳转页面时，若后端开启了同步服务，弹窗让用户选择
// "保留后台服务"（继续驻留同步）或"关闭服务"（POST /api/lifecycle/shutdown）。
// 勾选"不再提醒"后直接按所选方式执行，不再弹窗。
// 存储说明：不再提醒+行为选择存 localStorage（本机浏览器）；同时把行为
// 同步到后端内存（/api/lifecycle/close-pref），供"静默关闭"时判定。
// 限制：浏览器不允许页面脚本真正关闭用户手动开的标签页，且 beforeunload
// 里无法弹出自定义 UI；因此对"关闭"动作的可靠拦截是 beforeunload 原生确认框 +
// sendBeacon 发动作。完整体验 = 页面内"关闭界面"按钮（见下）。

const CG = {
  key: 'aipixel.closeGuard',            // localStorage: {noRemind, action}
  mask: () => document.getElementById('closeGuardMask'),
  noRemind: () => document.getElementById('cgNoRemind'),
  read() {
    try { return JSON.parse(localStorage.getItem(CG.key) || '{}'); } catch (e) { return {}; }
  },
  save(o) { localStorage.setItem(CG.key, JSON.stringify(Object.assign(CG.read(), o))); },
};

let __syncEnabledCache = null; // 最近一次 status 拿到的 sync_enabled，beforeunload 用（同步 fetch 太慢）

async function cgRefreshEnabled() {
  try {
    const d = await api('/api/sync/status');
    __syncEnabledCache = !!d.sync_enabled;
  } catch (e) { /* 服务已不可达视为无服务，不拦关闭 */ __syncEnabledCache = false; }
}

// 用户选择动作：记 localStorage + 通知后端（close-pref），keep 时什么也不做
async function cgChoose(action, noRemind) {
  CG.save({ action: action, noRemind: !!noRemind });
  try { await postJSON('/api/lifecycle/close-pref', { action: action }); } catch (e) {}
  if (action === 'stop') {
    // sendBeacon 也能用，但这里是显式按钮，直接 POST 拿到结果更稳
    try { await postJSON('/api/lifecycle/shutdown'); } catch (e) {}
    cgShowStopped();
  } else {
    cgHide();
    if (noRemind) {
      // 不再提醒 + 保留：下次关闭不再弹（原生确认也不再弹，见 beforeunload）
    }
  }
}

function cgShow() {
  const m = CG.mask(); if (!m) return;
  const saved = CG.read();
  CG.noRemind().checked = !!saved.noRemind;
  m.style.display = 'flex';
}
function cgHide() { const m = CG.mask(); if (m) m.style.display = 'none'; }

function cgShowStopped() {
  const dlg = document.querySelector('.cg-dialog');
  if (!dlg) return;
  dlg.innerHTML = '<h3>服务已停止</h3><p class="cg-text">后台同步服务已关闭，数据分析界面已不可用。可以直接关闭本页。</p>' +
    '<div class="cg-btns"><button class="btn primary" id="cgClosePageBtn">关闭本页</button></div>';
  const btn = document.getElementById('cgClosePageBtn');
  if (btn) btn.addEventListener('click', cgClosePage);
}

// 浏览器限制：window.close() 只对脚本 window.open 打开的窗口生效，
// 用户手动打开的标签页会被静默忽略。因此先尝试 close，再无条件把页面
// 替换为"安全提示页"——能关则关，关不掉用户也能看到明确提示。
function cgClosePage() {
  try { window.close(); } catch (e) {}
  // 给 window.close 一个极短的生效窗口；若页面还在（绝大多数情况），展示安全提示
  setTimeout(cgRenderSafeScreen, 250);
}

// 把整页替换为安全提示：停掉全部轮询/定时器，页面不再发任何请求。
function cgRenderSafeScreen() {
  if (__dataVerTimer) { clearInterval(__dataVerTimer); __dataVerTimer = null; }
  if (syncTimer) { clearInterval(syncTimer); syncTimer = null; }
  document.body.innerHTML =
    '<div class="safe-screen">' +
      '<div class="safe-card">' +
        '<div class="safe-ico">&#10003;</div>' +
        '<h2>服务已停止</h2>' +
        '<p>数据分析服务与后台同步均已关闭，本页面与服务器已无任何连接。</p>' +
        '<p class="dim">可以直接关闭此标签页。下次使用请重新运行 start-web.bat。</p>' +
      '</div>' +
    '</div>';
}

// 弹窗按钮
(function cgBind() {
  const keep = document.getElementById('cgKeep');
  if (!keep) return;
  keep.addEventListener('click', () => cgChoose('keep', CG.noRemind().checked));
  document.getElementById('cgStop').addEventListener('click', () => cgChoose('stop', CG.noRemind().checked));
  document.getElementById('cgCancel').addEventListener('click', cgHide);
  // 头部加"关闭界面"按钮：用户主动点 = 最可靠的入口，弹窗可完整展示
  const bar = document.querySelector('.controls');
  if (bar) {
    const b = document.createElement('button');
    b.id = 'btnCloseUI'; b.className = 'btn'; b.textContent = '关闭界面';
    b.style.marginLeft = '8px';
    b.addEventListener('click', async () => {
      await cgRefreshEnabled();
      if (!__syncEnabledCache) { window.close(); return; } // 没开同步：直接关，无需打扰
      const saved = CG.read();
      if (saved.noRemind && saved.action) { cgChoose(saved.action, true); return; }
      cgShow();
    });
    bar.appendChild(b);
  }
})();

// 浏览器关闭/刷新拦截：自定义弹窗在 beforeunload 里弹不出来，
// 只能给原生确认框。若用户已选"不再提醒+保留"则完全不拦；
// 若已选"不再提醒+关闭服务"则用 sendBeacon 静默停机，不拦。
// 默认（未设置）：有同步服务时拦一下，提示回页面用"关闭界面"按钮选择。
window.addEventListener('beforeunload', (e) => {
  if (!__syncEnabledCache) return;
  const saved = CG.read();
  if (saved.noRemind) {
    if (saved.action === 'stop') {
      navigator.sendBeacon('/api/lifecycle/shutdown', new Blob(['{}'], { type: 'application/json' }));
    }
    return; // keep 或已处理：不拦
  }
  e.preventDefault();
  e.returnValue = '后台同步服务仍在运行。如需选择保留/关闭服务，请留在页面用右上角"关闭界面"按钮。';
});

// 首次轮询后就知道 sync_enabled（pollDataVersion 每3s也会刷新缓存）
cgRefreshEnabled();
setInterval(cgRefreshEnabled, 3000);
