const state = { packages: [], current: null, ruleFilter: '', revisionPreview: null, freezePreview: null };

async function api(path, options = {}) {
  const response = await fetch('/api/v1' + path, { headers: { 'Content-Type': 'application/json' }, ...options });
  const data = await response.json();
  if (!response.ok) throw new Error(data.error || '请求失败');
  return data;
}

function esc(value) {
  return String(value ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}

async function load() {
  const query = new URLSearchParams();
  const status = document.querySelector('#status').value;
  const text = document.querySelector('#search').value.trim();
  if (status) query.set('status', status);
  if (text) query.set('q', text);
  if (state.ruleFilter) query.set('ruleCode', state.ruleFilter);
  const data = await api('/subtitle-packages?' + query);
  state.packages = data.packages || [];
  const s = data.stats;
  document.querySelector('#ruleFilter').innerHTML = state.ruleFilter ? `规则筛选：<strong>${esc(state.ruleFilter)}</strong> <button id="clearRule">清除</button>` : '';
  document.querySelector('#clearRule')?.addEventListener('click', () => { state.ruleFilter = ''; load(); });
  document.querySelector('#stats').textContent = s ? `总数 ${s.total} · 已交付 ${s.delivered} · 草稿 ${s.byStatus.draft || 0} · 审校中 ${s.byStatus.reviewing || 0} · 需要修订 ${s.byStatus.correction_required || 0} · 复审通过 ${s.byStatus.review_passed || 0} · 已冻结 ${s.byStatus.frozen || 0}` : '';
  document.querySelector('#packages').innerHTML = state.packages.map(p => `<div class="pkg" data-id="${esc(p.id)}"><strong>${esc(p.programTitle)}</strong> ${esc(p.episodeCode)} <span class="badge">${esc(p.status)}</span><div>版本 ${p.version} · ${p.cues.length} 条字幕</div></div>`).join('') || '<p>暂无匹配字幕包</p>';
  document.querySelectorAll('#packages .pkg[data-id]').forEach(item => item.onclick = () => show(item.dataset.id));
  if (state.current && !state.packages.some(p => p.id === state.current.id)) {
    state.current = null;
    document.querySelector('#detail').innerHTML = '';
  }
}

function actions(p) {
  if (p.status === 'prepared') return '<button data-action="check">运行质检</button>';
  if (p.status === 'reviewing') return p.needsQualityCheck ? '<button data-action="check">重新质检</button>' : '<button data-action="review">提交复审</button>';
  if (p.status === 'frozen') return '<button data-action="deliver">签发交付</button>';
  return '';
}

function findingRows(p) {
  return (p.findings || []).map(f => {
    const dispositionSelect = f.disposition === 'open' && p.status === 'reviewing' ? `<input type="checkbox" name="finding" value="${esc(f.id)}">` : '';
    const revisionSelect = (f.disposition === 'open' || f.disposition === 'question') && p.status === 'correction_required' ? `<input type="checkbox" name="revisionFinding" value="${esc(f.id)}">` : '';
    return `<div class="finding" data-cue="${esc(f.cueId)}">${dispositionSelect}${revisionSelect}<span class="source ${esc(f.source)}">${f.source === 'manual' ? '人工' : '自动'}</span><strong>${esc(f.ruleCode)}</strong> · ${esc(f.severity)} · ${esc(f.message)} · ${f.cueId ? `cue ${esc(f.cueId)}` : `${f.startMs}-${f.endMs} ms`} · ${esc(f.disposition)}</div>`;
  }).join('') || '<p>暂无发现</p>';
}

function manualFindingForm(p) {
  if (p.status !== 'reviewing') return '';
  const options = p.cues.map(c => `<option value="${esc(c.id)}">第 ${c.sequence} 条 · ${esc(c.text)}</option>`).join('');
  return `<section class="workarea"><h3>登记人工问题</h3><div class="formgrid"><label>字幕定位<select id="manualCue"><option value="">使用时间范围</option>${options}</select></label><label>开始毫秒<input id="manualStart" type="number" min="0"></label><label>结束毫秒<input id="manualEnd" type="number" min="1"></label><label>问题分类<input id="manualRule" placeholder="例如 WORDING"></label><label>严重级别<select id="manualSeverity"><option value="warning">warning</option><option value="error">error</option></select></label><label class="wide">问题描述<textarea id="manualMessage" rows="2"></textarea></label></div><button id="manualSubmit">登记问题</button></section>`;
}

function revisionForm(p) {
  if (p.status !== 'correction_required') return '';
  const rows = p.cues.map(c => `<tr class="revisionCue" data-cue="${esc(c.id)}"><td><input type="checkbox" class="revSelect"> ${c.sequence}</td><td><input class="revStart number" type="number" value="${c.startMs}"><input class="revEnd number" type="number" value="${c.endMs}"></td><td><input class="revSpeaker" value="${esc(c.speaker)}"></td><td><textarea class="revText" rows="2">${esc(c.text)}</textarea></td><td><input class="revHint" value="${esc(c.soundHint)}"></td></tr>`).join('');
  return `<section class="workarea"><h3>修订影响预检</h3><table><thead><tr><th>选择</th><th>时间码</th><th>说话人</th><th>文本</th><th>声音提示</th></tr></thead><tbody>${rows}</tbody></table><label>批次原因<input id="revisionReason" class="wideinput"></label><div class="toolbar"><button id="revisionPreview">预检影响</button><button id="revisionSubmit" disabled>提交修订</button></div><div id="revisionImpact"></div></section>`;
}

function freezeForm(p) {
  if (p.status !== 'review_passed') return '';
  return `<section class="workarea"><h3>冻结前母版预检</h3><button id="freezePreview">生成预检</button><div id="freezeResult"></div></section>`;
}

async function show(id) {
  const data = await api('/subtitle-packages/' + id);
  const p = data.package;
  const c = data.checklist || {};
  state.current = p;
  state.revisionPreview = null;
  state.freezePreview = null;
  const lastRevision = (data.events || []).filter(e => e.type === 'revision.created').at(-1)?.sequence || 0;
  const lastCheck = (data.events || []).filter(e => e.type === 'quality.checked').at(-1)?.sequence || 0;
  p.needsQualityCheck = lastRevision > lastCheck;
  const batch = p.status === 'reviewing' && p.findings.some(f => f.disposition === 'open') ? '<div class="toolbar"><button data-batch="resolved">批量接受</button><button data-batch="waived">批量豁免</button><button data-batch="question">批量提问</button></div>' : '';
  const revisions = (p.revisions || []).map(r => `<div class="record"><strong>${esc(r.id)}</strong> · 基准版本 ${r.baseVersion} · ${esc(r.reason)} · ${esc(r.submittedBy)}<div>关联发现：${(r.findingIds || []).map(esc).join('、') || '无'}</div><pre>${esc(JSON.stringify(r.changes, null, 2))}</pre></div>`).join('') || '<p>暂无修订</p>';
  document.querySelector('#detail').innerHTML = `<div class="panel"><h2>${esc(p.programTitle)} ${esc(p.episodeCode)}</h2><p>状态：<span class="badge">${esc(p.status)}</span>　版本：${p.version}</p><div class="toolbar">${actions(p)}</div>${p.status === 'draft' ? '<textarea id="importRaw" rows="4" placeholder="开始毫秒|结束毫秒|说话人|字幕文本|声音提示"></textarea><button id="preview">预览导入</button><button id="confirmImport">确认导入</button><pre id="previewOut"></pre>' : ''}<table><thead><tr><th>序号</th><th>时间</th><th>说话人</th><th>文本</th><th>声音提示</th></tr></thead><tbody>${p.cues.map(cue => `<tr id="cue-${esc(cue.id)}"><td>${cue.sequence}</td><td>${cue.startMs} - ${cue.endMs} ms</td><td>${esc(cue.speaker) || '-'}</td><td>${esc(cue.text)}</td><td>${esc(cue.soundHint) || '-'}</td></tr>`).join('')}</tbody></table><h3>质检汇总</h3><p>总计 ${c.total || 0} · 错误 ${c.errors || 0} · 警告 ${c.warnings || 0} · 未解决 ${(c.open || 0) + (c.question || 0)}</p><div id="findings">${findingRows(p)}</div>${batch}${manualFindingForm(p)}${revisionForm(p)}${freezeForm(p)}<h3>修订记录</h3>${revisions}${p.master ? `<h3>冻结母版</h3><p>${esc(p.master.summary)} · SHA-256 ${esc(p.master.checksum)} · <a href="/api/v1/subtitle-packages/${esc(p.id)}/master">下载 WebVTT</a></p>` : ''}${p.credential ? `<h3>交付凭据</h3><pre>${esc(JSON.stringify(p.credential, null, 2))}</pre>` : ''}<h3>审计时间线</h3><div class="toolbar"><input id="auditType" placeholder="事件类型"><input id="auditActor" placeholder="操作者"><input id="auditLimit" type="number" min="0" max="200" value="50"><button id="auditLoad">查询</button></div><div id="timeline"></div></div>`;
  document.querySelectorAll('[data-action]').forEach(button => button.onclick = () => act(button.dataset.action, p));
  document.querySelectorAll('[data-batch]').forEach(button => button.onclick = () => batchDisposition(p, button.dataset.batch));
  document.querySelector('#preview')?.addEventListener('click', () => previewImport(p));
  document.querySelector('#confirmImport')?.addEventListener('click', () => confirmImport(p));
  document.querySelector('#manualSubmit')?.addEventListener('click', () => addManualFinding(p));
  document.querySelector('#revisionPreview')?.addEventListener('click', () => previewRevision(p));
  document.querySelector('#revisionSubmit')?.addEventListener('click', () => submitRevision(p));
  document.querySelectorAll('.revisionCue input,.revisionCue textarea,input[name="revisionFinding"]').forEach(el => el.addEventListener('input', invalidateRevisionPreview));
  document.querySelector('#freezePreview')?.addEventListener('click', () => previewFreeze(p));
  document.querySelector('#auditLoad').onclick = () => loadAudit(p.id);
  renderTimeline(data.timeline || []);
}

function renderTimeline(items) {
  document.querySelector('#timeline').innerHTML = items.map(e => `<div>${e.sequence} · ${esc(e.label)} · ${esc(e.actor)} · ${esc(e.at)}</div>`).join('') || '暂无事件';
}

async function loadAudit(id) {
  const query = new URLSearchParams();
  const type = document.querySelector('#auditType').value.trim();
  const actor = document.querySelector('#auditActor').value.trim();
  const limit = document.querySelector('#auditLimit').value;
  if (type) query.set('type', type);
  if (actor) query.set('actor', actor);
  if (limit !== '') query.set('limit', limit);
  renderTimeline((await api('/audit/' + id + '?' + query)).timeline || []);
}

async function previewImport(p) {
  try {
    const raw = document.querySelector('#importRaw').value;
    const data = await api('/subtitle-packages/' + p.id + '/prepare-preview', { method: 'POST', body: JSON.stringify({ raw }) });
    document.querySelector('#previewOut').textContent = JSON.stringify(data.cues, null, 2);
  } catch (e) { alert(e.message); }
}

async function confirmImport(p) {
  try {
    const raw = document.querySelector('#importRaw').value;
    await api('/subtitle-packages/' + p.id + '/prepare', { method: 'POST', body: JSON.stringify({ expectedVersion: p.version, idempotencyKey: 'import-' + Date.now(), raw }) });
    await load(); await show(p.id);
  } catch (e) { alert(e.message); }
}

async function addManualFinding(p) {
  const cueId = document.querySelector('#manualCue').value;
  const body = { expectedVersion: p.version, idempotencyKey: 'manual-' + Date.now(), role: 'reviewer', cueId, ruleCode: document.querySelector('#manualRule').value.trim(), severity: document.querySelector('#manualSeverity').value, message: document.querySelector('#manualMessage').value.trim() };
  if (!cueId) {
    body.startMs = Number(document.querySelector('#manualStart').value);
    body.endMs = Number(document.querySelector('#manualEnd').value);
  }
  try {
    await api('/subtitle-packages/' + p.id + '/manual-findings', { method: 'POST', body: JSON.stringify(body) });
    await load(); await show(p.id);
  } catch (e) { alert(e.message); }
}

async function batchDisposition(p, disposition) {
  const ids = [...document.querySelectorAll('input[name="finding"]:checked')].map(input => input.value);
  if (!ids.length) return alert('请先选择待处置发现');
  const note = disposition === 'resolved' ? '' : (prompt('请输入处置说明') || '').trim();
  if (disposition !== 'resolved' && !note) return;
  try {
    await api('/subtitle-packages/' + p.id + '/findings', { method: 'POST', body: JSON.stringify({ expectedVersion: p.version, idempotencyKey: 'finding-' + Date.now(), role: 'reviewer', findings: ids.map(findingId => ({ findingId, disposition, resolutionNote: note })) }) });
    await load(); await show(p.id);
  } catch (e) { alert(e.message); }
}

function revisionPayload(p) {
  const changes = [...document.querySelectorAll('.revisionCue')].filter(row => row.querySelector('.revSelect').checked).map(row => {
    const before = p.cues.find(c => c.id === row.dataset.cue);
    return { cueId: before.id, before, after: { ...before, startMs: Number(row.querySelector('.revStart').value), endMs: Number(row.querySelector('.revEnd').value), speaker: row.querySelector('.revSpeaker').value, text: row.querySelector('.revText').value, soundHint: row.querySelector('.revHint').value } };
  });
  const findingIds = [...document.querySelectorAll('input[name="revisionFinding"]:checked')].map(input => input.value);
  return { changes, findingIds };
}

function invalidateRevisionPreview() {
  state.revisionPreview = null;
  const submit = document.querySelector('#revisionSubmit');
  if (submit) submit.disabled = true;
}

async function previewRevision(p) {
  try {
    const payload = revisionPayload(p);
    const data = await api('/subtitle-packages/' + p.id + '/revisions/preview', { method: 'POST', body: JSON.stringify(payload) });
    state.revisionPreview = { version: p.version, payload };
    const impact = data.preview;
    document.querySelector('#revisionImpact').innerHTML = `<div class="impact"><strong>字段差异</strong><pre>${esc(JSON.stringify(impact.diffs, null, 2))}</pre><p>可关联发现 ${impact.linkableFindings.length} 项 · 仍未覆盖 ${impact.uncoveredFindings.length} 项 · 时间码相邻字幕 ${impact.adjacentCues.length} 条</p>${impact.uncoveredFindings.map(f => `<div>${esc(f.ruleCode)} · ${esc(f.message)}</div>`).join('')}</div>`;
    document.querySelector('#revisionSubmit').disabled = false;
  } catch (e) { invalidateRevisionPreview(); alert(e.message); }
}

async function submitRevision(p) {
  if (!state.revisionPreview || state.revisionPreview.version !== p.version) return;
  const reason = document.querySelector('#revisionReason').value.trim();
  try {
    await api('/subtitle-packages/' + p.id + '/revisions', { method: 'POST', body: JSON.stringify({ expectedVersion: p.version, idempotencyKey: 'revision-' + Date.now(), role: 'editor', reason, ...state.revisionPreview.payload }) });
    await load(); await show(p.id);
  } catch (e) { alert(e.message); }
}

async function previewFreeze(p) {
  try {
    const data = await api('/subtitle-packages/' + p.id + '/freeze-preview');
    state.freezePreview = data.preview;
    document.querySelector('#freezeResult').innerHTML = `<p>${esc(data.preview.summary)} · ${data.preview.cueCount} 条 · SHA-256 ${esc(data.preview.checksum)}</p><pre class="masterPreview">${esc(data.preview.content)}</pre><button id="freezeConfirm">确认冻结</button>`;
    document.querySelector('#freezeConfirm').onclick = () => confirmFreeze(p);
  } catch (e) { alert(e.message); }
}

async function confirmFreeze(p) {
  if (!state.freezePreview) return;
  try {
    await api('/subtitle-packages/' + p.id + '/freeze', { method: 'POST', body: JSON.stringify({ expectedVersion: state.freezePreview.expectedVersion, expectedChecksum: state.freezePreview.checksum, idempotencyKey: 'freeze-' + Date.now(), role: 'delivery' }) });
    await load(); await show(p.id);
  } catch (e) { alert(e.message); }
}

async function act(action, p) {
  try {
    const endpoint = { check: 'quality-check', review: 'review', deliver: 'deliver' }[action];
    const roles = { check: 'reviewer', review: 'reviewer', deliver: 'delivery' };
    await api('/subtitle-packages/' + p.id + '/' + endpoint, { method: 'POST', body: JSON.stringify({ expectedVersion: p.version, idempotencyKey: action + '-' + Date.now(), role: roles[action] }) });
    await load(); await show(p.id);
  } catch (e) { alert(e.message); }
}

async function loadQualityStatistics() {
  const query = new URLSearchParams();
  const fields = [['from', '#statsFrom'], ['to', '#statsTo'], ['language', '#statsLanguage'], ['deliveryStandard', '#statsStandard']];
  fields.forEach(([key, selector]) => { const value = document.querySelector(selector).value.trim(); if (value) query.set(key, value); });
  try {
    const data = await api('/statistics/quality?' + query);
    const s = data.statistics;
    const rows = (s.findings || []).map(f => `<tr class="ruleRow" data-rule="${esc(f.ruleCode)}"><td>${esc(f.ruleCode)}</td><td>${esc(f.severity)}</td><td>${esc(f.source)}</td><td>${esc(f.disposition)}</td><td>${f.count}</td></tr>`).join('') || '<tr><td colspan="5">暂无匹配数据</td></tr>';
    document.querySelector('#qualityResults').innerHTML = `<div class="metricgrid"><div><strong>${s.packageCount}</strong><span>字幕包</span></div><div><strong>${s.deliveredCount}</strong><span>已交付</span></div><div><strong>${s.firstPassCount}</strong><span>首次通过</span></div><div><strong>${s.returnedCount}</strong><span>退回</span></div><div><strong>${s.reworkBatchCount}</strong><span>返修批次</span></div><div><strong>${s.revisionCount}</strong><span>修订记录</span></div></div><table><thead><tr><th>规则</th><th>级别</th><th>来源</th><th>处置</th><th>数量</th></tr></thead><tbody>${rows}</tbody></table>`;
    document.querySelectorAll('.ruleRow').forEach(row => row.onclick = () => { state.ruleFilter = row.dataset.rule; switchView('packages'); load(); });
  } catch (e) { alert(e.message); }
}

function switchView(view) {
  document.querySelector('#packageView').hidden = view !== 'packages';
  document.querySelector('#qualityView').hidden = view !== 'quality';
  document.querySelector('#packagesTab').classList.toggle('active', view === 'packages');
  document.querySelector('#qualityTab').classList.toggle('active', view === 'quality');
  if (view === 'quality') loadQualityStatistics();
}

document.querySelector('#new').onclick = async () => {
  const title = prompt('节目标题');
  if (!title) return;
  try {
    await api('/subtitle-packages', { method: 'POST', body: JSON.stringify({ programTitle: title, episodeCode: 'EP01', audioDurationMs: 60000, language: 'zh-CN', deliveryStandard: 'WebVTT WCAG' }) });
    await load();
  } catch (e) { alert(e.message); }
};
document.querySelector('#refresh').onclick = load;
document.querySelector('#clear').onclick = () => { document.querySelector('#status').value = ''; document.querySelector('#search').value = ''; state.ruleFilter = ''; load(); };
document.querySelector('#status').onchange = load;
document.querySelector('#search').oninput = (() => { let timer; return () => { clearTimeout(timer); timer = setTimeout(load, 250); }; })();
document.querySelector('#packagesTab').onclick = () => switchView('packages');
document.querySelector('#qualityTab').onclick = () => switchView('quality');
document.querySelector('#statsLoad').onclick = loadQualityStatistics;
load();
