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
}

// ===== 总览 =====
async function loadOverview() {
  const d = await api('/api/overview');
  $('#now').textContent = '更新于 ' + fmt.time(d.now) + ' · 当前小时 ' + d.cur_hour;
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
