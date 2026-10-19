'use strict';

// ---------------------------------------------------------------- 공통

const TOKEN = (location.hash.match(/t=([^&]+)/) || [])[1] || '';
const OS_LABEL = { mac: 'Mac', windows: 'Windows', linux: 'Linux' };
const INTERVALS = [5, 10, 15, 30, 60, 120, 360];

// 선 아이콘 (24×24, currentColor)
const svg = (d) => `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${d}</svg>`;
const ICONS = {
  folders: svg('<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>'),
  pcs: svg('<rect x="3" y="4" width="18" height="12" rx="2"/><path d="M8 20h8M12 16v4"/>'),
  settings: svg('<path d="M4 6h9M17 6h3M4 12h3M11 12h9M4 18h11M19 18h1"/><circle cx="15" cy="6" r="2"/><circle cx="9" cy="12" r="2"/><circle cx="17" cy="18" r="2"/>'),
  bell: svg('<path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9"/><path d="M10.3 21a1.94 1.94 0 0 0 3.4 0"/>'),
  collapse: svg('<path d="M11 17l-5-5 5-5M18 17l-5-5 5-5"/>'),
  expand: svg('<path d="M13 17l5-5-5-5M6 17l5-5-5-5"/>'),
  x: svg('<path d="M18 6 6 18M6 6l12 12"/>'),
};

const $ = (sel, root = document) => root.querySelector(sel);
const $$ = (sel, root = document) => Array.from(root.querySelectorAll(sel));
const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
const clone = (v) => JSON.parse(JSON.stringify(v));

async function api(path, body) {
  const opt = { headers: { 'X-GS-Token': TOKEN } };
  if (body !== undefined) {
    opt.method = 'POST';
    opt.headers['Content-Type'] = 'application/json';
    opt.body = JSON.stringify(body);
  }
  let res;
  try {
    res = await fetch(path, opt);
  } catch (e) {
    const err = new Error(t('GodseonSync 에 연결할 수 없습니다. 프로그램이 꺼졌다면 다시 실행하세요.'));
    err.offline = true;
    throw err;
  }
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    const err = new Error(data.error || res.statusText);
    err.status = res.status;
    err.data = data;
    throw err;
  }
  return data;
}

function ago(iso) {
  if (!iso) return '';
  const d = new Date(iso);
  if (isNaN(d)) return iso;
  const s = (Date.now() - d.getTime()) / 1000;
  if (s < 60) return t('방금');
  if (s < 3600) return t('{n}분 전', { n: Math.floor(s / 60) });
  if (s < 86400) return t('{n}시간 전', { n: Math.floor(s / 3600) });
  if (s < 86400 * 7) return t('{n}일 전', { n: Math.floor(s / 86400) });
  return d.toLocaleDateString(LANG === 'en' ? 'en-US' : 'ko-KR');
}

function fullTime(iso) {
  if (!iso) return '';
  const d = new Date(iso);
  return isNaN(d) ? iso : d.toLocaleString(LANG === 'en' ? 'en-US' : 'ko-KR');
}

function toast(msg, kind = '') {
  const el = document.createElement('div');
  el.className = `toast ${kind}`;
  el.textContent = msg;
  $('#toasts').appendChild(el);
  setTimeout(() => el.remove(), kind === 'err' ? 7000 : 3500);
}

// copyText 는 글을 클립보드에 넣는다. 클릭 처리 안에서 기다리기(await) 전에 불러야 한다 (WKWebView 는 사용자 동작 중에만 복사된다).
// 전용 창(WKWebView·WebView2)과 브라우저 모두에서 되는 execCommand 를 먼저 쓰고, 안 되면 Clipboard API.
function copyText(text) {
  const ta = document.createElement('textarea');
  ta.value = text;
  ta.setAttribute('readonly', '');
  ta.style.cssText = 'position:fixed;top:0;left:0;opacity:0;pointer-events:none';
  const prev = document.activeElement;
  document.body.appendChild(ta);
  ta.select();
  ta.setSelectionRange(0, text.length);
  let ok = false;
  try {
    ok = document.execCommand('copy');
  } catch (e) { /* 아래 Clipboard API 로 */ }
  ta.remove();
  prev?.focus?.();
  if (ok) return Promise.resolve();
  return navigator.clipboard ? navigator.clipboard.writeText(text) : Promise.reject(new Error('clipboard'));
}

// modal 은 버튼을 누르면 { action, form } 으로 끝나는 Promise 를 돌려준다. 바깥을 누르거나 Esc 면 action=null.
function modal({ title, body = '', actions = [{ label: t('확인'), kind: 'primary', value: 'ok' }], onOpen }) {
  return new Promise((resolve) => {
    const root = $('#modal-root');
    root.innerHTML = `
      <div class="modal-backdrop">
        <div class="modal" role="dialog" aria-modal="true" aria-label="${esc(title)}">
          <h2>${esc(title)}</h2>
          <form class="modal-body" onsubmit="return false">${body}</form>
          <div class="modal-actions">
            ${actions.map((a, i) => `<button class="btn ${a.kind || ''}" data-i="${i}">${esc(a.label)}</button>`).join('')}
          </div>
        </div>
      </div>`;
    const close = (action) => {
      const form = Object.fromEntries(new FormData($('.modal-body', root)));
      root.innerHTML = '';
      document.removeEventListener('keydown', onKey);
      resolve({ action, form });
    };
    const onKey = (e) => {
      if (e.key === 'Escape') close(null);
      if (e.key === 'Enter' && e.target.tagName === 'INPUT') {
        const primary = actions.findIndex((a) => a.kind === 'primary');
        if (primary >= 0) close(actions[primary].value);
      }
    };
    document.addEventListener('keydown', onKey);
    $('.modal-backdrop', root).addEventListener('mousedown', (e) => {
      if (e.target.classList.contains('modal-backdrop')) close(null);
    });
    $$('.modal-actions button', root).forEach((b) => b.addEventListener('click', () => close(actions[+b.dataset.i].value)));
    if (onOpen) onOpen($('.modal', root), close);
    const first = $('input, select, textarea', root);
    if (first) first.focus();
  });
}

async function confirmBox(title, text, okLabel = t('확인'), danger = false) {
  const { action } = await modal({
    title,
    body: `<p class="muted">${text}</p>`,
    actions: [{ label: t('취소'), value: null }, { label: okLabel, kind: danger ? 'primary danger-fill' : 'primary', value: 'ok' }],
  });
  return action === 'ok';
}

// ---------------------------------------------------------------- 상태

const S = {
  overview: null, // /api/overview
  status: null, // /api/status
  remote: null, // /api/config 결과 (cached 면 이 PC 에 저장해 둔 마지막 설정)
  draft: null, // 편집 중인 공용 설정
  draftBase: null, // 편집을 시작한 시점의 설정 — 저장할 때 충돌 검사 기준으로 보낸다
  dirty: false,
  settingsSync: { state: 'idle', at: '', error: '' }, // 드라이브에서 setting.json 받기: idle / syncing / ok / error
  wasRunning: false,
  open: new Set(), // 펼쳐 둔 details
  setup: { probe: null, probeError: '', name: '', adopt: '', autostart: true, busy: false },
  conn: { mode: '', values: {}, migrating: false }, // 저장소 연결 입력 칸 (처음 설정·설정 탭이 같이 쓴다). 보안 비밀은 서버가 돌려주지 않는다
  logText: '',
  logFolder: loadPref('gs-log-folder', ''), // 설정 › 로그에서 보는 로그: '' 면 전체 로그, 아니면 폴더 ID
  logFolders: [], // 폴더 로그가 있는 폴더 ID (/api/log — 설정에서 빠졌어도 로그가 남은 폴더)
  expanded: loadExpanded(), // 펼쳐 둔 폴더 ID (저장한 폴더는 기본으로 접힘)
  tab: loadTab(), // 지금 화면(왼쪽 메뉴): folders / pcs / settings
  settingsTab: loadPref('gs-settings-tab', 'general').replace(/^storage$/, 'mode'), // 설정 화면 안의 탭 (예전 이름 storage)
  drawerCollapsed: loadPref('gs-drawer', innerWidth < 760 ? 'collapsed' : 'expanded') === 'collapsed',
  nt: { open: false, items: [], unread: 0, filter: 'all', opened: new Set(), loaded: false, failed: false }, // 알림 센터 (loaded: 목록을 한 번이라도 받았는지)
};

const SETTINGS_TABS = [['general', '일반'], ['exclude', '제외 규칙'], ['mode', '모드'], ['pc', '이 PC'], ['log', '로그']];

function loadPref(key, def) {
  try {
    return localStorage.getItem(key) || def;
  } catch (e) {
    return def;
  }
}

function savePref(key, value) {
  try {
    localStorage.setItem(key, value);
  } catch (e) { /* 저장 못 해도 이번 화면에서는 그대로 쓴다 */ }
}

const TABS = ['folders', 'pcs', 'settings'];

// ---------------------------------------------------------------- 저장소 방식(모드)

// 모드별 연결 입력 칸. 서버의 internal/storage 와 짝을 맞춘다 (새 모드는 양쪽에 더한다).
//   fields: 늘 보이는 칸, advanced: '고급 설정' 안의 칸, describe: 설정 탭에 보여 줄 지금 연결 설명
const MODES = {
  gdrive: {
    connectLabel: '구글 계정으로 연결',
    browser: true, // 브라우저에서 로그인한다
    summary: '구글 계정으로 로그인합니다. 내 드라이브의 GodseonSync 폴더를 씁니다.',
    fields: [
      { key: 'client_id', label: 'OAuth 클라이언트 ID', placeholder: '123456-abc….apps.googleusercontent.com' },
      { key: 'client_secret', label: '클라이언트 보안 비밀', secret: true },
    ],
    // 앱(빌드)에 클라이언트가 들어 있으면 클라이언트 칸은 고급 설정으로 숨긴다 — 로그인만 하면 된다
    builtinOptional: true,
    advanced: [
      { key: 'scope', label: '권한 범위', options: [['drive.file', '이 앱이 만든 파일만 (권장)'], ['drive', '드라이브 전체 (다른 앱으로 만든 폴더도 쓸 때, 로그인 때 경고)']] },
    ],
    hint: '직접 만든 구글 OAuth 클라이언트(데스크톱 앱)의 ID 와 보안 비밀입니다. 비워 두면 rclone 공용 클라이언트를 쓰는데, 전 세계가 같이 써서 드라이브 응답이 느리거나 실패할 수 있습니다. 만드는 방법은 README 의 \'구글 OAuth 클라이언트\' 를 보세요.',
    builtinHint: '구글 계정으로 로그인만 하면 됩니다. 직접 만든 OAuth 클라이언트를 쓰려면 고급 설정에 넣으세요.',
    describe: (info) => {
      const scope = info.scope === 'drive.file' ? t('이 앱이 만든 파일만') : t('드라이브 전체');
      if (info.builtin) return `<div class="notice info">${t('앱에 들어 있는 GodseonSync 클라이언트로 연결되어 있습니다 · 권한 범위 {scope}', { scope })}</div>`;
      return info.client_id
        ? `<div class="notice info">${t('직접 만든 OAuth 클라이언트로 연결되어 있습니다: {id} · 권한 범위 {scope}', { id: `<code>${esc(info.client_id)}</code>`, scope })}</div>`
        : `<div class="notice warn">${t('rclone 공용 OAuth 클라이언트로 연결되어 있습니다 (권한 범위 {scope}). 전 세계가 같이 써서 드라이브 응답이 느리거나 실패할 수 있습니다. 아래에 직접 만든 클라이언트를 넣고 다시 연결하세요.', { scope })}</div>`;
    },
  },
  sftp: {
    connectLabel: '연결',
    browser: false,
    summary: 'NAS·서버에 SFTP(SSH)로 접속합니다. 홈 폴더(또는 정한 폴더) 아래 GodseonSync 폴더를 씁니다.',
    fields: [
      { key: 'host', label: '호스트', placeholder: 'nas.local / 192.168.0.10' },
      { key: 'port', label: '포트', placeholder: '22' },
      { key: 'user', label: '사용자' },
      { key: 'auth', label: '로그인 방식', options: [['password', '비밀번호'], ['key', '키 파일']] },
      { key: 'pass', label: '비밀번호', secret: true, showIf: (v) => (v.auth || 'password') === 'password' },
      { key: 'key_file', label: '키 파일', placeholder: '~/.ssh/id_ed25519', showIf: (v) => v.auth === 'key' },
      { key: 'key_file_pass', label: '키 암호 (있을 때만)', secret: true, showIf: (v) => v.auth === 'key' },
      { key: 'path', label: '폴더 위치', placeholder: '비우면 홈 폴더 · 예) upload, /volume1/sync' },
    ],
    hint: '폴더 위치 아래에 GodseonSync 폴더를 만듭니다. 홈 폴더 맨 위에 쓸 수 없는 서버(chroot)는 쓸 수 있는 하위 폴더를 넣으세요. 비밀번호·키 암호는 이 PC 의 rclone.conf 에 가려서 저장됩니다. 모든 PC 가 같은 서버·같은 사용자·같은 폴더 위치로 연결해야 합니다.',
    describe: (info, where) => `<div class="notice info">${t('SFTP 서버에 연결되어 있습니다: {where} · {auth}', { where: `<code>${esc(where)}</code>`, auth: info.auth === 'key' ? t('키 파일') : t('비밀번호') })}</div>`,
  },
};

// 저장소 이름: 화면 문구의 {store}(드라이브 / SFTP), {name}(구글 드라이브 / SFTP), 맨 위 폴더(내 드라이브 / 홈 폴더)
const storeMode = () => S.overview?.connection?.mode || S.conn.mode || 'gdrive';
const storeWord = () => (storeMode() === 'sftp' ? 'SFTP' : t('드라이브'));
const storeName = () => (storeMode() === 'sftp' ? 'SFTP' : t('구글 드라이브'));
const storeRoot = () => (storeMode() === 'sftp' ? (S.overview?.connection?.info?.path || t('홈 폴더')) : t('내 드라이브'));

// 지금 모드가 앱에 들어 있는 클라이언트를 쓸 수 있는지 (그러면 클라이언트 칸은 고를 때만)
const builtinClient = () => !!(MODES[connMode()]?.builtinOptional && S.overview?.custom_client);

// 화면에 보일 모드 목록: 서버가 아는 것 중 입력 칸이 정의된 것
const modeList = () => (S.overview?.modes || []).filter((m) => MODES[m.key]);

// 지금 입력 칸의 모드 (고르지 않았으면 이 PC 의 연결, 그것도 없으면 첫 모드)
function connMode() {
  const c = S.conn;
  if (!c.mode || !MODES[c.mode]) c.mode = S.overview?.connection?.mode || modeList()[0]?.key || 'gdrive';
  return c.mode;
}

function renderConnField(f) {
  if (f.showIf && !f.showIf(S.conn.values)) return '';
  const v = S.conn.values[f.key] ?? '';
  if (f.options) {
    return `<label class="field"><span>${esc(t(f.label))}</span>
      <select data-conn="${f.key}">${f.options.map(([val, text]) => `<option value="${val}" ${(v || f.options[0][0]) === val ? 'selected' : ''}>${esc(t(text))}</option>`).join('')}</select></label>`;
  }
  return `<label class="field"><span>${esc(t(f.label))}</span><input class="mono" data-conn="${f.key}" value="${esc(v)}"
    ${f.secret ? 'type="password" autocomplete="off"' : 'autocomplete="off" spellcheck="false"'} placeholder="${esc(f.placeholder || '')}"></label>`;
}

// 모드 카드: 구글 드라이브 / SFTP 중 고른다 (처음 설정·설정 › 모드).
function renderModeCards() {
  const modes = modeList();
  if (modes.length < 2) return '';
  const cur = S.overview?.connection?.mode;
  return `<div class="mode-cards" role="radiogroup" aria-label="${esc(t('저장소 방식'))}">${modes.map((m) => `
    <button class="mode-card ${connMode() === m.key ? 'active' : ''}" role="radio" aria-checked="${connMode() === m.key}" data-action="pick-mode" data-mode="${m.key}">
      <span class="mc-title">${esc(m.label)}${configured() && cur === m.key ? ` <span class="pill ok">${t('지금 쓰는 중')}</span>` : S.overview?.saved?.[m.key] ? ` <span class="pill idle">${t('저장된 연결')}</span>` : ''}</span>
      <span class="mc-sub">${esc(t(MODES[m.key].summary))}</span>
    </button>`).join('')}</div>`;
}

// 고른 모드의 입력 칸. withAdvanced 면 '고급 설정' 칸도 보인다.
// 앱에 클라이언트가 들어 있으면 클라이언트 칸은 '고급 설정'(설정 탭은 '다른 OAuth 클라이언트 쓰기') 안에 둔다.
function renderConnForm({ withAdvanced }) {
  const m = MODES[connMode()];
  const builtin = builtinClient();
  const main = builtin ? [] : m.fields;
  const adv = [...(builtin ? m.fields : []), ...(withAdvanced ? m.advanced || [] : [])];
  const hint = builtin ? (withAdvanced ? m.builtinHint : '') : m.hint; // 설정 탭은 위의 연결 설명으로 충분하다
  const advHtml = adv.length ? `
    <details class="sub" data-open-key="conn-adv" ${S.open.has('conn-adv') ? 'open' : ''}>
      <summary>${withAdvanced ? t('고급 설정') : t('다른 OAuth 클라이언트 쓰기')}</summary>${adv.map(renderConnField).join('')}
    </details>` : '';
  return `${main.map(renderConnField).join('')}
    ${hint ? `<p class="field-hint" style="margin-top:6px">${esc(t(hint))}</p>` : ''}${advHtml}`;
}

// 연결 작업(브라우저 로그인 등)이 도는 동안 보여 줄 안내
function renderConnectJob(job) {
  return `
    <p>${job.message ? esc(job.message) : MODES[connMode()]?.browser === false ? t('연결하는 중…') : t('브라우저에서 로그인하고 <b>허용</b>을 누르세요. 끝나면 이 화면이 자동으로 넘어갑니다.')}</p>
    <div class="row wrap" style="margin-top:10px">
      ${job.url && !job.message ? t('<button class="btn" data-action="connect-open">브라우저가 안 열렸으면 여기를 누르세요</button>') : ''}
      ${job.finishing ? '' : t('<button class="btn ghost" data-action="connect-cancel">취소</button>')}
    </div>`;
}

function connectBody() {
  const m = MODES[connMode()];
  const values = {};
  for (const f of [...m.fields, ...(m.advanced || [])]) values[f.key] = (S.conn.values[f.key] ?? (f.options ? f.options[0][0] : '')).trim();
  return { mode: S.conn.mode, options: values };
}

function loadTab() {
  try {
    const t = localStorage.getItem('gs-tab');
    return TABS.includes(t) ? t : 'folders';
  } catch (e) {
    return 'folders';
  }
}

// 폴더를 펼쳐 둔 상태는 이 화면(브라우저·설정 창)에만 기억한다
function loadExpanded() {
  try {
    return new Set(JSON.parse(localStorage.getItem('gs-expanded-folders') || '[]'));
  } catch (e) {
    return new Set();
  }
}

function saveExpanded() {
  try {
    localStorage.setItem('gs-expanded-folders', JSON.stringify([...S.expanded]));
  } catch (e) { /* 저장 못 해도 이번 화면에서는 그대로 쓴다 */ }
}

// 새로 추가해서 아직 저장하지 않은 폴더는 펼쳐서 보여 준다
const isExpanded = (f) => (f.id ? S.expanded.has(f.id) : !f._collapsed);

const me = () => S.overview?.settings?.client_id;
const configured = () => !!S.overview?.settings?.configured;

async function loadOverview() {
  S.overview = await api('/api/overview');
  const st = S.overview.settings;
  if (!st.language) {
    // 처음: 시스템(브라우저) 언어로 정해 이 PC 설정에 저장한다 (트레이·알림 문구도 따라간다)
    st.language = (navigator.language || '').toLowerCase().startsWith('ko') ? 'ko' : 'en';
    api('/api/background', { language: st.language }).catch(() => {});
  }
  setLang(st.language);
  if (!S.setup.name) S.setup.name = st.client_name || S.overview.hostname;
  $('#app-version').textContent = `v${S.overview.app_version}`;
  document.title = `GodseonSync — ${st.client_name}`;
}

// applyRemote 는 받은 설정을 화면 상태에 넣는다. 고치는 중(dirty)이면 편집 내용과 편집 기준(draftBase)은 그대로 둔다.
function applyRemote(r, { keepDraft = false } = {}) {
  if (!r.statuses) r.statuses = {};
  S.remote = r;
  if (r.config && (!keepDraft || !S.dirty || !S.draft)) {
    S.draft = clone(r.config);
    S.draftBase = clone(r.config);
    setDirty(false);
  }
}

// 폴더 목록·공통 제외 규칙만 비교한다 (서버의 SameFolders 와 같은 기준)
function sameFolders(a, b) {
  return !!a && !!b && JSON.stringify([a.folders, a.exclude]) === JSON.stringify([b.folders, b.exclude]);
}

// 고치는 사이 다른 PC 가 폴더 목록을 바꿨는지 (그대로 저장하면 서버가 충돌로 막는다)
const remoteChanged = () => S.dirty && S.remote?.config && S.draftBase && !S.remote.cached && !sameFolders(S.remote.config, S.draftBase);

// loadCached 는 드라이브에 가지 않고 이 PC 가 마지막으로 받아 둔 설정으로 화면을 바로 연다. 캐시가 없으면 false.
async function loadCached() {
  try {
    const r = await api('/api/config?cached=1');
    if (!r.config) return false;
    applyRemote(r);
    S.settingsSync = { state: 'idle', at: r.cached_at || '', error: '' };
    return true;
  } catch (e) {
    return false;
  }
}

// loadRemote 는 드라이브에서 최신 setting.json 과 PC 상태를 받는다 (= 설정 동기화). 수 초 걸릴 수 있다.
async function loadRemote({ keepDraft = false } = {}) {
  S.settingsSync = { ...S.settingsSync, state: 'syncing' };
  updateSettingsSync();
  let r;
  try {
    r = await api('/api/config');
  } catch (e) {
    r = { offline: true, cached: true, error: e.message, config: S.remote?.config || null, statuses: S.remote?.statuses || {} };
  }
  applyRemote(r, { keepDraft });
  S.settingsSync = r.offline
    ? { state: 'error', at: r.cached_at || S.settingsSync.at, error: r.error || '' }
    : { state: 'ok', at: r.synced_at || new Date().toISOString(), error: '' };
  updateSettingsSync();
}

// syncSettings 는 화면을 연 채로 드라이브의 최신 설정을 받는다. 고치는 중이면 다시 그리지 않고 상태 표시만 바꾼다
// (다시 그리면 한글 입력 중인 글자가 끊긴다).
async function syncSettings({ manual = false } = {}) {
  if (!configured() || S.settingsSync.state === 'syncing') return;
  await loadRemote({ keepDraft: true });
  if (S.dirty) updateSettingsSync();
  else render();
  if (manual) {
    if (S.settingsSync.state === 'ok') toast(t('{store}에서 최신 설정을 받았습니다.', { store: storeWord() }));
    else toast(t('설정을 받지 못했습니다: {err}', { err: S.settingsSync.error }), 'err');
  }
}

function settingsSyncHtml() {
  const ss = S.settingsSync;
  const when = ss.at ? ago(ss.at) : '';
  if (ss.state === 'syncing') {
    return `<span class="muted"><span class="spinner"></span> ${t('최신 설정 받는 중…')}${S.remote?.cached && when ? ` ${t('(지금은 {when} 받은 설정)', { when: esc(when) })}` : ''}</span>`;
  }
  if (ss.state === 'error') {
    return `<span class="err-text" title="${esc(ss.error)}">${when ? t('{store}에 연결할 수 없어 마지막으로 받은 설정({when})을 보여 줍니다', { store: storeWord(), when: esc(when) }) : t('{store}에 연결할 수 없어 마지막으로 받은 설정을 보여 줍니다', { store: storeWord() })}</span>`;
  }
  return when ? `<span class="muted">${t('설정: {when} 받음', { when: `<span title="${esc(fullTime(ss.at))}">${esc(when)}</span>` })}</span>` : '';
}

function remoteChangedHtml() {
  return remoteChanged()
    ? t('<div class="notice err">다른 PC 에서 폴더 설정이 바뀌었습니다. 저장하면 충돌로 막히니 <b>되돌리기</b>를 누른 뒤 다시 고치세요.</div>')
    : '';
}

function updateSettingsSync() {
  const el = $('#settings-sync');
  if (el) el.innerHTML = settingsSyncHtml();
  const warn = $('#settings-warn');
  if (warn) warn.innerHTML = remoteChangedHtml();
  $$('[data-action="sync-settings"]').forEach((b) => { b.disabled = S.settingsSync.state === 'syncing'; });
}

async function pollStatus() {
  try {
    S.status = await api('/api/status');
  } catch (e) {
    S.status = null;
    renderTopbar(e.offline ? t('연결 끊김') : t('오류'));
    return;
  }
  const running = !!S.status.running;
  if (S.status.unread !== S.nt.unread) {
    S.nt.unread = S.status.unread || 0;
    if (S.nt.open) loadNotices();
  }
  renderTopbar();
  updateFolderStatus();
  const job = S.status.connect;
  const before = S.overview.connect;
  if (job?.state !== before?.state || job?.message !== before?.message) {
    S.overview.connect = job;
    if (job?.state === 'done') {
      await loadOverview();
      if (configured() && before?.state === 'running') {
        S.conn.values = {};
        if (S.conn.migrating || S.conn.moving) {
          toast(S.conn.moving ? t('저장소를 바꿨습니다. 지금 폴더마다 한 번 합치며 새 저장소에 올립니다.') : t('옮겼습니다. 지금 폴더마다 한 번 합치며 새 GodseonSync 폴더에 다시 올립니다.'));
          api('/api/sync', {}).then(() => setTimeout(pollStatus, 400)).catch((e) => toast(e.message, 'err'));
        } else {
          toast(t('새 정보로 다시 연결했습니다.'));
        }
        S.conn.migrating = false;
        S.conn.moving = false;
        S.conn.prefilled = {};
        syncSettings();
      }
    }
    render();
  }
  if (running || S.wasRunning) refreshLog();
  const finished = S.wasRunning && !running;
  S.wasRunning = running;
  if (finished) syncSettings(); // 동기화가 끝나면 다른 PC 상태도 새로 받는다
}

function setDirty(v) {
  S.dirty = v;
  $('#savebar').hidden = !v;
}

// ---------------------------------------------------------------- 상단바

function overallState() {
  if (!S.overview) return { cls: 'idle', text: t('불러오는 중') };
  if (!configured()) return { cls: 'idle', text: t('설정 필요') };
  const st = S.status;
  if (!st) return { cls: 'err', text: t('연결 끊김') };
  if (st.running) return { cls: 'run', text: st.running.folder ? t('동기화 중: {name}', { name: st.running.folder }) : t('동기화 중') };
  if (st.status?.last_error) return { cls: 'err', text: t('오류') };
  const folders = Object.values(st.status?.folders || {});
  if (folders.some((f) => f.last_error)) return { cls: 'err', text: t('일부 폴더 오류') };
  if (folders.some((f) => f.needs_choice)) return { cls: 'warn', text: t('결정 필요') };
  if (S.overview.settings.paused) return { cls: 'warn', text: t('자동 동기화 일시 정지') };
  if (!st.status?.last_run) return { cls: 'idle', text: t('아직 동기화 전') };
  return { cls: 'ok', text: t('정상 · {when}', { when: ago(st.status.last_run) }) };
}

function renderTopbar(override) {
  const right = $('#topbar-right');
  if (!S.overview) {
    right.innerHTML = '';
    return;
  }
  const state = override ? { cls: 'err', text: override } : overallState();
  const unread = S.nt.unread;
  right.innerHTML = `
    <span class="chip" title="${esc(t('이 PC 의 ID: {id}', { id: me() }))}">${t('이 PC: {name}', { name: esc(S.overview.settings.client_name) })}</span>
    <span class="pill ${state.cls}">${esc(state.text)}</span>
    ${configured() ? `<button class="btn primary small" data-action="sync-all" ${S.status?.running ? 'disabled' : ''}>${t('지금 동기화')}</button>` : ''}
    <button class="icon-btn ${S.nt.open ? 'active' : ''}" data-action="toggle-notices" title="${esc(t('알림'))}" aria-label="${esc(t('알림'))}" aria-expanded="${S.nt.open}">
      ${ICONS.bell}${unread ? `<span class="badge">${unread > 99 ? '99+' : unread}</span>` : ''}</button>`;
}

// ---------------------------------------------------------------- 처음 설정 화면

function renderSetup() {
  const ov = S.overview;
  const su = S.setup;
  const job = ov.connect;
  const connected = ov.connected && job?.state !== 'running';
  const probe = su.probe;

  let step1;
  if (job?.state === 'running') {
    step1 = renderConnectJob(job);
  } else if (connected) {
    step1 = `
      <p>${t('{name}에 연결되었습니다.', { name: esc(ov.connection?.label || t('저장소')) })}</p>
      <div class="row" style="margin-top:10px"><button class="btn small" data-action="connect">${t('다른 계정으로 다시 연결')}</button></div>`;
  } else {
    step1 = `
      <p>${t('GodseonSync 가 파일을 올리고 받을 저장소를 연결합니다. 로그인 정보는 이 PC 에만 저장됩니다.')}</p>
      ${job?.state === 'error' ? `<div class="notice err" style="margin-top:10px">${t('연결하지 못했습니다: {err}', { err: esc(job.message) })}</div>` : ''}
      ${renderModeCards()}
      ${renderConnForm({ withAdvanced: true })}
      <div class="row" style="margin-top:10px"><button class="btn primary" data-action="connect">${esc(t(MODES[connMode()].connectLabel))}</button></div>`;
  }

  let probeHtml = '';
  if (connected && !probe && su.probeError) {
    probeHtml = `<div class="notice err" style="margin-top:12px">${esc(su.probeError)}</div>
      <div class="row" style="margin-top:10px"><button class="btn" data-action="probe">${t('다시 시도')}</button></div>`;
  } else if (connected && !probe) {
    probeHtml = `<p class="muted" style="margin-top:10px">${t('{name}에 GodseonSync 폴더를 준비하는 중…', { name: storeName() })}</p>`;
  } else if (probe && !probe.created) {
    const pcs = probe.pcs || [];
    probeHtml = `
      <div class="notice info" style="margin-top:12px">${t('기존 설정({path})을 찾았습니다 — 폴더 {n}개{names}, PC {pcs}대', { path: `<code>${storeRoot()}/${esc(probe.setting)}</code>`, n: probe.folders.length, names: probe.folders.length ? ` (${esc(probe.folders.slice(0, 5).join(', '))}${probe.folders.length > 5 ? ' …' : ''})` : '', pcs: pcs.length })}</div>
      ${pcs.length ? `<div class="radio-list">
        <label><input type="radio" name="adopt" value="" ${!su.adopt ? 'checked' : ''} data-setup="adopt"> <span><b>${t('새 PC 로 등록')}</b> <span class="muted small">— ${t('처음 설치하는 PC')}</span></span></label>
        ${pcs.map((c) => `
          <label><input type="radio" name="adopt" value="${esc(c.id)}" ${su.adopt === c.id ? 'checked' : ''} data-setup="adopt">
            <span>${t('{name} 이어받기', { name: `<b>${esc(c.name)}</b>` })} <span class="os-tag">${esc(OS_LABEL[c.os] || '?')}</span>
            <span class="muted small">${c.last_seen ? t('마지막 연결 {when}', { when: esc(ago(c.last_seen)) }) : ''}</span></span></label>`).join('')}
      </div>
      <p class="field-hint" style="margin-top:6px">${t('이 PC 를 다시 설치한 경우에만 이어받기를 고르세요. 이어받으면 그 PC 의 경로 설정을 그대로 씁니다.')}</p>` : ''}`;
  } else if (probe) {
    const fileScope = ov.connection?.mode === 'gdrive' && ov.connection.info?.scope === 'drive.file';
    probeHtml = `<div class="notice info" style="margin-top:12px">${storeMode() === 'sftp' ? t('{root}에 {path} 을 만들었습니다. 다른 PC 에서도 같은 SFTP 서버·사용자로 연결하면 이 설정에 참여합니다.', { root: storeRoot(), path: `<code>${esc(probe.setting)}</code>` }) : t('{root}에 {path} 을 만들었습니다. 다른 PC 에서도 같은 구글 계정으로 로그인하면 이 설정에 참여합니다.', { root: storeRoot(), path: `<code>${esc(probe.setting)}</code>` })}</div>
      ${fileScope ? `<p class="field-hint" style="margin-top:6px">${t("다른 PC 에서 이미 GodseonSync 를 쓰고 있었는데 새로 만들어졌다면, 그 폴더는 다른 클라이언트나 '드라이브 전체' 범위로 만들어져 이 범위('이 앱이 만든 파일만')로는 보이지 않는 것입니다. 그 PC 를 새 버전으로 업데이트한 뒤 설정 › 저장소 연결에서 '이 앱이 만든 파일만으로 옮기기' 를 먼저 하고, 이 PC 는 '다른 계정으로 다시 연결' 하세요.")}</p>` : ''}`;
  }

  const step2Done = !!probe;
  return `
    <div class="hero">
      <h1>${t('GodseonSync 시작하기')}</h1>
      <p>${t('여러 PC 의 폴더를 구글 드라이브나 SFTP 서버를 거쳐 폴더 구조 그대로 동기화합니다.')}</p>
    </div>
    <div class="card steps">
      <div class="step ${connected ? 'done' : 'active'}">
        <div class="step-num">${connected ? '✓' : '1'}</div>
        <div class="step-body"><h3>${t('저장소 연결')}</h3>${step1}</div>
      </div>
      <div class="step ${connected ? (step2Done ? 'done' : 'active') : 'locked'}">
        <div class="step-num">${step2Done ? '✓' : '2'}</div>
        <div class="step-body">
          <h3>${t('저장소의 GodseonSync 폴더와 이 PC 이름')}</h3>
          <p>${t('설정은 {path} 에 저장되고, 같은 저장소에 연결한 모든 PC 가 이 파일을 함께 씁니다.', { path: `<code>${storeRoot()}/GodseonSync/setting.json</code>` })}</p>
          <label class="field"><span>${t('이 PC 이름')}</span><input data-setup="name" value="${esc(su.name)}" placeholder="${esc(ov.hostname)}"></label>
          <p class="field-hint">${t('PC 마다 따로 등록됩니다. 같은 Windows 라도 PC 가 다르면 다른 이름을 쓰세요.')}</p>
          ${probeHtml}
        </div>
      </div>
      <div class="step ${step2Done ? 'active' : 'locked'}">
        <div class="step-num">3</div>
        <div class="step-body">
          <h3>${t('백그라운드 실행')}</h3>
          <label class="switch" style="margin-top:10px"><input type="checkbox" data-setup="autostart" ${su.autostart ? 'checked' : ''}><span class="track"></span>
            <span>${t('로그인할 때 자동으로 실행해서 백그라운드에서 동기화')}</span></label>
          ${ov.autostart.problem ? `<div class="notice warn" style="margin-top:10px">${esc(ov.autostart.problem)}</div>` : ''}
          <div class="row" style="margin-top:14px"><button class="btn primary" data-action="finish-setup" ${su.busy ? 'disabled' : ''}>${probe && !probe.created ? t('참여하기') : t('시작하기')}</button></div>
        </div>
      </div>
    </div>`;
}

// ---------------------------------------------------------------- 메인 화면

// clientList 는 setting.json 의 PC 목록에 각 PC 가 올린 상태(.status)를 붙인 것이다.
function clientList() {
  const statuses = S.remote?.statuses || {};
  const pcs = [...(S.remote?.config?.pcs || [])];
  // 설정에는 경로가 있는데 PC 목록에 없는 경우도 보여 준다
  for (const f of S.draft?.folders || []) {
    for (const id of Object.keys(f.paths || {})) {
      if (!pcs.some((p) => p.id === id)) pcs.push({ id, name: id, os: '' });
    }
  }
  if (!pcs.some((p) => p.id === me())) pcs.push({ id: me(), name: S.overview.settings.client_name, os: S.overview.os });
  return pcs
    .map((p) => ({ ...(statuses[p.id] || {}), ...p }))
    .sort((a, b) => (a.id === me() ? -1 : b.id === me() ? 1 : (a.name || '').localeCompare(b.name || '', 'ko')));
}

function folderState(f) {
  const mine = (f.paths || {})[me()];
  if (!f.id) return { cls: 'idle', text: t('저장 필요'), info: t('저장하면 동기화를 시작합니다.') };
  if (!mine) return { cls: 'idle', text: t('이 PC 미사용'), info: t('이 PC 경로를 입력하면 이 PC 에서도 동기화합니다.') };
  const run = S.status?.running;
  if (run && run.folder === f.name) return { cls: 'run', text: t('동기화 중'), info: t('동기화 중…') };
  const fs = S.status?.status?.folders?.[f.id];
  if (fs?.needs_choice) {
    const b = (choice, label, kind = '') => `<button class="btn small ${kind}" data-action="choose-folder" data-id="${esc(f.id)}" data-choice="${choice}">${label}</button>`;
    return {
      cls: 'warn', text: t('결정 필요'), choice: true,
      info: `<span>${fs.diff
        ? t('이 PC 와 {store}가 달라 멈췄습니다 — 이 PC 에만 {onlyLocal}개, {store}에만 {onlyDrive}개, 내용이 다른 파일 {different}개. 어느 쪽으로 맞출지 고르세요.',
          { store: storeWord(), onlyLocal: fs.diff.only_local, onlyDrive: fs.diff.only_drive, different: fs.diff.different })
        : t('이 PC 에 {local}개, {store}에 {drive}개 파일이 있어 멈췄습니다. 어느 쪽으로 맞출지 고르세요.', { local: fs.local_files, store: storeWord(), drive: fs.drive_files })}</span>
        ${fs.diff ? diffSamplesHtml(fs.diff) : ''}
        <span class="row wrap" style="margin-top:6px">${b('drive', t('{store} → 이 PC 덮어쓰기', { store: storeWord() }))}${b('local', t('이 PC → {store} 덮어쓰기', { store: storeWord() }))}${b('merge', t('합치기 (둘 다 남김)'), 'ghost')}</span>`,
    };
  }
  if (!fs || !fs.last_run) return { cls: 'idle', text: t('대기'), info: t('아직 동기화하지 않았습니다.') };
  if (fs.last_error) {
    return {
      cls: 'err', text: t('오류'), error: fs.last_error, tooMany: !!fs.too_many_deletes,
      info: `<span class="err-text">${esc(fs.last_error)}</span>${fs.last_ok ? ` · ${t('마지막 성공 {when}', { when: esc(ago(fs.last_ok)) })}` : ''}`,
    };
  }
  return { cls: 'ok', text: t('정상'), info: t('마지막 동기화 {when}', { when: `<span title="${esc(fullTime(fs.last_ok))}">${esc(ago(fs.last_ok))}</span>` }) };
}

// 결정 필요 폴더의 차이 예시 (이 PC 에만 / 저장소에만 / 내용이 다른 파일 — 몇 개만)
function diffSamplesHtml(d) {
  const rows = [[t('이 PC 에만'), d.only_local, d.only_local_samples], [t('{store}에만', { store: storeWord() }), d.only_drive, d.only_drive_samples], [t('내용이 다름'), d.different, d.different_samples]]
    .filter(([, n]) => n > 0)
    .map(([label, n, list]) => `<div><b>${esc(label)}</b> ${n} — ${(list || []).map((p) => `<code>${esc(p)}</code>`).join(' ')}${n > (list || []).length ? ' …' : ''}</div>`);
  return rows.length ? `<details class="sub diff-samples"><summary>${t('다른 파일 보기')}</summary><div class="small" style="display:grid;gap:4px;margin-top:6px">${rows.join('')}</div></details>` : '';
}

function renderFolder(f, i, clients) {
  const others = clients.filter((c) => c.id !== me());
  const setCount = others.filter((c) => (f.paths || {})[c.id]).length;
  const st = folderState(f);
  const key = f.id || `new-${i}`;
  const tooMany = st.tooMany; // 언어와 상관없이 엔진이 남긴 표시로 본다
  const open = isExpanded(f);
  return `
    <div class="card folder ${f.id ? '' : 'new'} ${open ? '' : 'collapsed'}" data-folder="${i}">
      <div class="folder-head" data-action="toggle-folder" data-f="${i}" title="${open ? t('접기') : t('펼치기')}">
        <button class="chev" type="button" data-action="toggle-folder" data-f="${i}" aria-expanded="${open}" aria-label="${open ? t('접기') : t('펼치기')}"></button>
        ${open
          ? `<input class="name-input" data-f="${i}" data-k="name" value="${esc(f.name)}" placeholder="${esc(t('폴더 이름'))}" aria-label="${esc(t('폴더 이름'))}">`
          : `<span class="folder-name">${esc(f.name || t('(이름 없음)'))}</span>`}
        <span class="pill ${st.cls}" id="fstat-${i}">${esc(st.text)}</span>
      </div>
      <div class="folder-body">
      <label class="field"><span>${t('{name} 경로', { name: storeName() })}</span>
        <div class="input-group"><input class="mono" data-f="${i}" data-k="drive_path" value="${esc(f.drive_path)}" placeholder="GodseonSync/${esc(t('폴더 이름'))}">
        <button class="btn" data-action="browse-drive" data-target="folder-${i}">${t('찾아보기')}</button></div></label>
      <label class="field"><span>${t('이 PC 경로')}</span>
        <div class="input-group"><input class="mono" data-f="${i}" data-p="${esc(me())}" value="${esc((f.paths || {})[me()] || '')}" placeholder="${esc(t('비워 두면 이 PC 에서는 동기화하지 않습니다'))}">
        <button class="btn" data-action="pick-local" data-f="${i}">${t('찾아보기')}</button></div></label>
      <details class="sub other-paths" data-open-key="other-${key}" ${S.open.has(`other-${key}`) ? 'open' : ''}>
        <summary>${t('다른 PC 경로')} (${setCount}/${others.length})</summary>
        ${others.length ? others.map((c) => {
          const rep = (c.folders || {})[f.id];
          const dot = rep?.last_error ? 'err' : rep?.last_ok ? 'ok' : '';
          const title = rep?.last_error ? t('오류: {err}', { err: rep.last_error }) : rep?.last_ok ? t('마지막 동기화 {when}', { when: fullTime(rep.last_ok) }) : t('아직 동기화 기록 없음');
          return `
          <label class="field"><span class="client-label" title="${esc(title)}"><span class="dot ${dot}"></span><span class="nm">${esc(c.name)}</span>${c.os ? `<span class="os-tag">${esc(OS_LABEL[c.os] || c.os)}</span>` : ''}</span>
            <input class="mono" data-f="${i}" data-p="${esc(c.id)}" value="${esc((f.paths || {})[c.id] || '')}" placeholder="${c.os === 'windows' ? t('예) D:\\문서') : t('예) /Users/이름/문서')} — ${esc(t('비우면 이 PC 는 동기화 안 함'))}"></label>`;
        }).join('') : `<p class="muted small" style="margin:8px 0 0">${t('다른 PC 가 아직 없습니다. 다른 PC 에 GodseonSync 를 설치하고 같은 구글 계정으로 로그인하면 여기에 나타납니다.')}</p>`}
      </details>
      <details class="sub" data-open-key="ex-${key}" ${S.open.has(`ex-${key}`) ? 'open' : ''}>
        <summary>${t('이 폴더만 제외할 항목')} (${(f.exclude || []).length})</summary>
        <textarea class="mono" data-f="${i}" data-k="exclude" placeholder="${esc(t('한 줄에 하나씩. 예)'))}&#10;node_modules/&#10;*.tmp">${esc((f.exclude || []).join('\n'))}</textarea>
        <p class="field-hint">${t('이름 끝에 / 를 붙이면 폴더, * 는 아무 글자. 공통 제외 규칙은 설정 › 제외 규칙에 있습니다.')}</p>
      </details>
      <div class="folder-foot">
        <div class="info" id="finfo-${i}">${st.info}</div>
        ${f.id && (f.paths || {})[me()] ? `
          <button class="btn small" data-action="sync-folder" data-id="${esc(f.id)}">${t('지금 동기화')}</button>
          <button class="btn small ghost" data-action="resync-folder" data-id="${esc(f.id)}" title="${esc(t('양쪽 파일을 합칩니다. 아무것도 지우지 않고, 같은 파일은 더 최신 것을 남깁니다.'))}">${t('합치기')}</button>
          <button class="btn small ghost" data-action="dryrun-folder" data-id="${esc(f.id)}" title="${esc(t('실제로 바꾸지 않고 바뀔 내용만 로그에 남깁니다.'))}">${t('미리보기')}</button>
          <button class="btn small ghost" data-action="folder-log" data-id="${esc(f.id)}" title="${esc(t('이 폴더의 동기화 기록(오간 파일과 결과)을 봅니다.'))}">${t('로그')}</button>
          ${tooMany ? `<button class="btn small ghost danger" data-action="force-folder" data-id="${esc(f.id)}">${t('강제 실행')}</button>` : ''}` : ''}
        <button class="btn small ghost danger" data-action="remove-folder" data-f="${i}">${t('삭제')}</button>
      </div>
      </div>
    </div>`;
}

function renderClients(clients) {
  const rows = clients.map((c) => {
    const isMe = c.id === me();
    const errs = Object.values(c.folders || {}).filter((r) => r.last_error).length;
    let status = '<span class="muted">-</span>';
    if (errs) status = `<span class="row"><span class="dot err"></span>${t('오류 {n}개', { n: errs })}</span>`;
    else if (c.last_seen) status = `<span class="row"><span class="dot ok"></span>${t('정상')}</span>`;
    const auto = c.paused ? t('일시 정지') : c.autostart ? t('켜짐 · {n}분', { n: c.interval_minutes || '?' }) : t('꺼짐');
    return `
      <tr>
        <td><b>${esc(c.name)}</b>${isMe ? t('<span class="me-tag">이 PC</span>') : ''}
          ${c.hostname && c.hostname !== c.name ? `<div class="muted small">${esc(c.hostname)}</div>` : ''}</td>
        <td>${esc(OS_LABEL[c.os] || c.os || '-')}</td>
        <td>${status}</td>
        <td class="muted small">${esc(auto)}</td>
        <td class="muted small" title="${esc(fullTime(c.last_seen))}">${esc(c.last_seen ? ago(c.last_seen) : '-')}</td>
        <td style="text-align:right">${isMe
          ? t('<button class="btn small ghost" data-action="rename-me">이름 바꾸기</button>')
          : `<button class="btn small ghost danger" data-action="remove-client" data-id="${esc(c.id)}" data-name="${esc(c.name)}">${t('목록에서 빼기')}</button>`}</td>
      </tr>`;
  }).join('');
  return `
    <div class="card" style="padding:6px 12px; overflow-x:auto">
      <table class="table">
        <thead><tr><th>${t('이름')}</th><th>OS</th><th>${t('상태')}</th><th>${t('자동 실행')}</th><th>${t('마지막 연결')}</th><th></th></tr></thead>
        <tbody>${rows}</tbody>
      </table>
    </div>`;
}

function renderMain() {
  const ov = S.overview;
  const r = S.remote || {};
  const cfg = S.draft;
  const clients = clientList();
  const notices = [];
  if (r.offline) notices.push(`<div class="notice err">${t('{name}에서 설정을 받지 못했습니다: {err}', { name: storeName(), err: esc(r.error) })}${cfg ? t(' — 마지막으로 받은 설정을 보여 줍니다.') : ''} <button class="btn small" data-action="sync-settings">${t('다시 시도')}</button></div>`);
  if (r.warnings?.length) notices.push(`<div class="notice warn">${t('확인이 필요합니다')}<ul>${r.warnings.map((w) => `<li>${esc(w)}</li>`).join('')}</ul></div>`);
  if (S.status?.status?.last_error) notices.push(`<div class="notice err">${t('마지막 동기화가 중단됐습니다: {err}', { err: esc(S.status.status.last_error) })}</div>`);
  if (ov.autostart.problem) notices.push(`<div class="notice warn">${esc(ov.autostart.problem)}</div>`);

  if (!cfg) {
    return `${notices.join('')}<div class="empty">${t('설정을 불러오지 못했습니다.')} <button class="btn small" data-action="reload">${t('다시 시도')}</button></div>`;
  }

  let body;
  if (S.tab === 'pcs') body = renderPCsTab(clients);
  else if (S.tab === 'settings') body = renderSettingsTab(cfg);
  else body = renderFoldersTab(cfg, clients);
  const title = { folders: t('폴더'), pcs: t('등록 PC'), settings: t('설정') }[S.tab] || t('폴더');

  return `
    <div class="content-head">
      <h1>${esc(title)}</h1>
      <div class="content-head-right">
        <span class="small" id="settings-sync">${settingsSyncHtml()}</span>
        <button class="btn ghost small" data-action="sync-settings" title="${esc(t('{name}의 GodseonSync/setting.json 을 다시 받아옵니다', { name: storeName() }))}" ${S.settingsSync.state === 'syncing' ? 'disabled' : ''}>${t('설정 동기화')}</button>
      </div>
    </div>
    ${notices.join('')}
    <div id="settings-warn">${remoteChangedHtml()}</div>
    ${body}`;
}

// 왼쪽 메뉴: 접으면 아이콘만, 펼치면 아이콘 + 이름. 처음 설정 화면에서는 숨긴다.
function renderDrawer() {
  const nav = $('#drawer');
  const cfg = S.draft;
  if (!S.overview || !configured() || !cfg) {
    nav.hidden = true;
    return;
  }
  const folderAlert = cfg.folders.some((f) => folderState(f).cls === 'err' || folderState(f).choice);
  const items = [
    { key: 'folders', label: t('폴더'), count: cfg.folders.length, alert: folderAlert },
    { key: 'pcs', label: t('등록 PC'), count: clientList().length },
    { key: 'settings', label: t('설정') },
  ];
  const c = S.drawerCollapsed;
  nav.hidden = false;
  nav.className = `drawer ${c ? 'collapsed' : ''}`;
  nav.setAttribute('aria-label', t('메뉴'));
  nav.innerHTML = `
    <button class="nav-item drawer-toggle" data-action="toggle-drawer" title="${esc(c ? t('메뉴 펼치기') : t('메뉴 접기'))}" aria-expanded="${!c}">
      ${c ? ICONS.expand : ICONS.collapse}<span class="nav-label">${t('메뉴 접기')}</span></button>
    ${items.map((it) => `
      <button class="nav-item ${S.tab === it.key ? 'active' : ''}" data-action="tab" data-tab="${it.key}" title="${esc(it.label)}" aria-current="${S.tab === it.key ? 'page' : 'false'}">
        ${ICONS[it.key]}<span class="nav-label">${esc(it.label)}</span>
        ${it.count !== undefined ? `<span class="nav-count">${it.count}</span>` : ''}
        ${it.key === 'folders' ? `<span class="nav-alert tab-alert" ${it.alert ? '' : 'hidden'}></span>` : ''}
      </button>`).join('')}`;
}

function renderFoldersTab(cfg, clients) {
  return `
    <section class="section">
      <div class="section-head">
        <p>${t('폴더마다 {name} 경로와 PC 별 경로를 정합니다. 바꾼 뒤 아래 <b>{store}에 저장</b>을 누르면 모든 PC 에 반영됩니다.', { name: storeName(), store: storeWord() })}</p>
        <div class="row">
          ${cfg.folders.length ? `<button class="btn ghost small" data-action="toggle-all">${cfg.folders.some(isExpanded) ? t('모두 접기') : t('모두 펼치기')}</button>` : ''}
          <button class="btn" data-action="add-folder">+ ${t('폴더 추가')}</button>
        </div>
      </div>
      ${cfg.folders.length ? cfg.folders.map((f, i) => renderFolder(f, i, clients)).join('') : `
        <div class="empty">${t('아직 동기화 폴더가 없습니다.')}<div style="margin-top:10px"><button class="btn primary" data-action="add-folder">+ ${t('첫 폴더 추가')}</button></div></div>`}
    </section>`;
}

function renderPCsTab(clients) {
  return `
    <section class="section">
      <div class="section-head">
        <p>${t('같은 구글 계정으로 로그인한 PC 들입니다. OS 가 같아도 PC 마다 따로 등록되고, 각 PC 는 동기화할 때마다 여기에 상태를 올립니다. 새 PC 는 그 PC 에 GodseonSync 를 설치하고 로그인하면 나타납니다.')}</p>
      </div>
      ${renderClients(clients)}
    </section>`;
}

function renderSettingsTab(cfg) {
  const ov = S.overview;
  const st = ov.settings;
  const r = S.remote || {};
  const tab = SETTINGS_TABS.some(([k]) => k === S.settingsTab) ? S.settingsTab : 'general';
  let body;
  if (tab === 'general') {
    body = `
    <section class="section">
      <div class="section-head"><div><h2>${t('언어')}${LANG === 'en' ? '' : ' / Language'}</h2></div></div>
      <div class="card"><div class="row"><span>${t('화면·알림 언어')}</span>
        <select data-bg="language" style="width:auto">${Object.entries(LANGS).map(([k, v]) => `<option value="${k}" ${LANG === k ? 'selected' : ''}>${v}</option>`).join('')}</select></div></div>
    </section>
    <section class="section">
      <div class="section-head"><div><h2>${t('백그라운드 실행')}</h2><p>${ov.os === 'mac' ? t('GodseonSync 는 메뉴 막대 아이콘으로 실행되고, 정해진 주기마다 동기화합니다.') : t('GodseonSync 는 작업 표시줄 알림 영역 아이콘으로 실행되고, 정해진 주기마다 동기화합니다.')}</p></div></div>
      <div class="card stack">
        <label class="switch"><input type="checkbox" data-bg="autostart" ${ov.autostart.enabled ? 'checked' : ''}><span class="track"></span><span>${t('로그인할 때 자동으로 실행')}</span></label>
        <label class="switch"><input type="checkbox" data-bg="paused" ${st.paused ? 'checked' : ''}><span class="track"></span><span>${t('자동 동기화 일시 정지')}</span></label>
        <div class="row"><span>${t('동기화 주기')}</span>
          <select data-bg="interval_minutes" style="width:auto">
            ${[...new Set([...INTERVALS, st.interval_minutes])].sort((a, b) => a - b).map((m) => `<option value="${m}" ${m === st.interval_minutes ? 'selected' : ''}>${m < 60 ? t('{n}분마다', { n: m }) : t('{n}시간마다', { n: m / 60 })}</option>`).join('')}
          </select></div>
      </div>
    </section>`;
  } else if (tab === 'exclude') {
    body = `
    <section class="section">
      <div class="section-head"><div><h2>${t('공통 제외 규칙')}</h2><p>${t('모든 폴더·모든 PC 에 적용됩니다. 한 줄에 하나씩. 이름 끝에 / 를 붙이면 폴더, * 는 아무 글자. rclone 필터 문법(<code>- 패턴</code>)도 쓸 수 있습니다. 바꾼 뒤 <b>{store}에 저장</b>을 누릅니다.', { store: storeWord() })}</p></div></div>
      <textarea class="mono" data-global="exclude" rows="10">${esc((cfg.exclude || []).join('\n'))}</textarea>
    </section>`;
  } else if (tab === 'mode') {
    body = renderModeTab();
  } else if (tab === 'pc') {
    body = `
    <section class="section">
      <div class="section-head"><div><h2>${t('저장소와 이 PC')}</h2></div></div>
      <div class="card small">
        <div>${t('공용 설정')}: <code>${storeRoot()}/${esc(st.root)}/setting.json</code> · ${t('PC 별 상태')}: <code>${storeRoot()}/${esc(st.root)}/.status/</code></div>
        <div class="muted" style="margin-top:4px">${t('설정 버전 {n}', { n: esc(r.config?.revision ?? '-') })}${r.config?.updated_by ? ` · ${t('{who} 가 {when} 수정', { who: esc(r.config.updated_by), when: esc(ago(r.config.updated_at)) })}` : ''}</div>
        <div class="muted" style="margin-top:4px">${t('이 PC')}: ${esc(st.client_name)} (<code>${esc(st.client_id)}</code>) · ${t('데이터 폴더')} <code>${esc(ov.data_dir)}</code></div>
        <div class="row wrap" style="margin-top:12px">
          <button class="btn small" data-action="reset">${t('처음 설정 다시 하기')}</button>
          <button class="btn small" data-action="disconnect">${t('구글 계정 연결 해제')}</button>
          <span class="spacer"></span>
          <button class="btn small danger" data-action="quit">${t('GodseonSync 종료')}</button>
        </div>
      </div>
    </section>`;
  } else {
    const choices = logChoices();
    if (S.logFolder && !choices.some((c) => c.id === S.logFolder)) S.logFolder = ''; // 지워진 폴더
    const cur = choices.find((c) => c.id === S.logFolder);
    const pick = (id, label) => `<button class="${S.logFolder === id ? 'active' : ''}" role="tab" aria-selected="${S.logFolder === id}" data-action="log-folder" data-id="${esc(id)}">${label}</button>`;
    body = `
    <section class="section">
      <div class="section-head"><div><h2>${t('로그')}</h2>
        <p>${cur ? t('{name} 폴더의 동기화 기록입니다. 오간 파일과 결과를 남깁니다.', { name: esc(cur.name) }) : t('GodseonSync 시작·종료, 설정 변경, 폴더마다 동기화 결과 한 줄을 남깁니다. 오간 파일은 폴더를 고르면 보입니다.')}</p></div>
        <div class="row"><button class="btn small ghost danger" data-action="clear-log">${t('로그 지우기')}</button><button class="btn small" data-action="refresh-log">${t('새로고침')}</button></div></div>
      <div class="log-picker" role="tablist" aria-label="${esc(t('볼 로그'))}">${pick('', t('전체'))}${choices.map((c) => pick(c.id, esc(c.name))).join('')}</div>
      <pre class="log" id="log">${logHtml(S.logText)}</pre>
      <p class="field-hint">${t('↑ 올림: 이 PC → {store} · ↓ 받음: {store} → 이 PC · − 지움 · ⚠ 확인 필요 · ✗ 실패', { store: storeWord() })}</p>
    </section>`;
  }
  return `
    <div class="tabbar">
      <div class="tabs" role="tablist">
        ${SETTINGS_TABS.map(([k, label]) => `
          <button class="tab ${tab === k ? 'active' : ''}" role="tab" aria-selected="${tab === k}" data-action="settings-tab" data-tab="${k}">${t(label)}</button>`).join('')}
      </div>
    </div>
    ${body}`;
}

// 설정 › 모드: 지금 저장소 + 모드 카드 + 고른 모드의 입력 칸.
// 같은 모드면 '다시 연결'(새 로그인이 확인된 뒤에만 바뀜, switchRemote), 다른 모드면 '이 저장소로 바꾸기'(switchStorage).
function renderModeTab() {
  const ov = S.overview;
  const conn = ov.connection;
  const job = ov.connect;
  const sel = connMode();
  const same = sel === conn?.mode;
  // 지금 쓰는 모드는 보안 비밀이 아닌 값(호스트 등)을 미리 채운다
  if (same && conn?.info && !S.conn.prefilled?.[sel]) {
    S.conn.prefilled = { ...(S.conn.prefilled || {}), [sel]: true };
    for (const f of MODES[sel].fields) if (!f.secret && conn.info[f.key] && !S.conn.values[f.key]) S.conn.values[f.key] = conn.info[f.key];
  }
  let body;
  if (job?.state === 'running') {
    body = renderConnectJob(job);
  } else {
    const now = !ov.connected
      ? `<div class="notice err">${t('저장소에 연결되어 있지 않습니다. 동기화가 실패합니다. 아래에서 다시 연결하세요.')}</div>`
      : conn && MODES[conn.mode] ? MODES[conn.mode].describe(conn.info || {}, conn.where || '') : '';
    const migrate = same && conn?.mode === 'gdrive' && conn.info?.scope === 'drive' ? `
      <div class="notice warn" style="margin-top:4px">
        <b>${t("권한 범위를 '이 앱이 만든 파일만'으로 옮기세요 (권장).")}</b> ${t("지금은 '드라이브 전체' 라서 구글 로그인 때 '확인되지 않은 앱' 경고가 뜨고, GodseonSync 가 드라이브의 다른 파일까지 볼 수 있습니다. 옮기면 경고가 없어지고 GodseonSync 가 만든 파일만 봅니다.")}
        <div class="row" style="margin-top:8px"><button class="btn small" data-action="migrate-scope">${t('이 앱이 만든 파일만으로 옮기기')}</button></div>
      </div>` : '';
    const errTitle = S.conn.migrating ? t('옮기지 못했습니다') : S.conn.moving ? t('저장소를 바꾸지 못했습니다') : t('다시 연결하지 못했습니다');
    const saved = !same && conn ? ov.saved?.[sel] : null;
    const savedHtml = saved ? `
      <div class="notice info">${saved.where && saved.where !== saved.label
        ? t('이 PC 에 저장해 둔 {name} 연결이 있습니다: {where}', { name: esc(saved.label), where: `<code>${esc(saved.where)}</code>` })
        : t('이 PC 에 저장해 둔 {name} 연결이 있습니다.', { name: esc(saved.label) })}
        <div class="row" style="margin-top:8px"><button class="btn primary small" data-action="restore-storage">${t('저장된 연결로 바꾸기')}</button></div>
      </div>` : '';
    const form = renderConnForm({ withAdvanced: false });
    body = `${now}
      ${job?.state === 'error' ? `<div class="notice err">${errTitle}: ${esc(job.message)}</div>` : ''}
      ${migrate}
      ${renderModeCards()}
      ${savedHtml}
      ${saved ? `<details class="sub" data-open-key="new-conn" ${S.open.has('new-conn') ? 'open' : ''}><summary>${t('새로 연결해서 바꾸기')}</summary>${form}` : form}
      ${same || !conn
        ? `<p class="field-hint">${t('다시 연결하는 동안에도 지금 연결로 동기화가 계속되고, 새 연결에서 같은 GodseonSync 설정이 보일 때만 바뀝니다. 폴더 설정은 그대로입니다.')}</p>
      <div class="row" style="margin-top:10px"><button class="btn primary small" data-action="reconnect">${builtinClient() && !S.conn.values.client_id ? t('다시 연결') : t('입력한 정보로 다시 연결')}</button></div>`
        : `<p class="field-hint">${t('지금 저장소의 내용과 연결은 그대로 두고 이 PC 만 새 저장소로 바꿉니다 (나중에 로그인 없이 돌아올 수 있습니다). 새 저장소에 GodseonSync 설정이 없으면 지금 설정을 복사해 만들고, 파일은 다음 동기화 때 폴더마다 한 번 합치며 올라갑니다.')}</p>
      <div class="row" style="margin-top:10px"><button class="btn ${saved ? '' : 'primary'} small" data-action="move-storage">${saved ? t('입력한 정보로 새로 연결해서 바꾸기') : t('이 저장소로 바꾸기')}</button></div>${saved ? '</details>' : ''}`}`;
  }
  return `
    <section class="section">
      <div class="section-head"><div><h2>${t('저장소 모드')}</h2><p>${t('파일을 올리고 받을 곳과 로그인 정보입니다. 이 PC 에만 저장됩니다. 모든 PC 가 같은 저장소를 써야 합니다.')}</p></div></div>
      <div class="card stack">${body}</div>
    </section>`;
}

// 폴더 선택 창. 윈도우 전용 창에서는 창 프로세스가 설정 창에 붙은(모달) 탐색기 창으로 띄운다 (gsPickFolder, window_windows.go).
// 그 밖에는 에이전트가 띄운다. 에이전트가 띄우면 윈도우에서는 설정 창 뒤에 숨는다 (백그라운드 프로세스는 앞으로 못 나온다).
function pickFolder(initial) {
  if (typeof window.gsPickFolder !== 'function') return api('/api/pick-folder', { initial });
  return new Promise((resolve, reject) => {
    window.gsFolderPicked = (r) => (r.error ? reject(new Error(r.error)) : resolve(r));
    window.gsPickFolder(initial, t('동기화할 폴더를 고르세요')).then((ok) => {
      if (!ok) reject(new Error(t('폴더 선택 창이 이미 열려 있습니다.')));
    }, reject);
  });
}

// ---------------------------------------------------------------- 알림 센터

async function loadNotices() {
  try {
    const r = await api('/api/notices');
    S.nt.items = r.items || [];
    S.nt.unread = r.unread || 0;
    S.nt.loaded = true;
    S.nt.failed = false;
  } catch (e) {
    S.nt.failed = true; // 불러온 적이 있으면 그 목록을 그대로 둔다 (다음 번에 다시)
  }
  renderNoticePanel();
  renderTopbar();
}

function renderNoticePanel() {
  const panel = $('#notice-panel');
  if (!S.nt.open) {
    panel.hidden = true;
    return;
  }
  const items = S.nt.filter === 'unread' ? S.nt.items.filter((n) => !n.read) : S.nt.items;
  panel.hidden = false;
  panel.innerHTML = `
    <div class="np-head">
      <h2>${t('알림')}</h2>
      <button class="btn ghost small" data-action="notices-read-all" ${S.nt.unread ? '' : 'disabled'}>${t('모두 읽음')}</button>
      <button class="btn ghost small danger" data-action="notices-delete-all" ${S.nt.items.length ? '' : 'disabled'}>${t('모두 지우기')}</button>
    </div>
    <div class="np-filter">
      <button class="${S.nt.filter === 'all' ? 'active' : ''}" data-action="notices-filter" data-filter="all">${t('전체')} ${S.nt.items.length}</button>
      <button class="${S.nt.filter === 'unread' ? 'active' : ''}" data-action="notices-filter" data-filter="unread">${t('안 읽음')} ${S.nt.unread}</button>
    </div>
    <div class="np-list">
      ${!S.nt.loaded ? `<div class="np-empty">${S.nt.failed // 받기 전에는 '알림이 없습니다' 가 아니다 (배지 숫자와 어긋나 보인다)
        ? `${t('알림을 불러오지 못했습니다.')} <button class="btn small" data-action="notices-reload">${t('다시 시도')}</button>`
        : t('불러오는 중…')}</div>` : items.length ? items.map((n) => `
        <div class="np-item ${n.read ? '' : 'unread'} ${S.nt.opened.has(n.id) ? 'open' : ''}" data-action="notice-open" data-id="${esc(n.id)}">
          <span class="lv ${esc(n.level)}"></span>
          <div>
            <div class="tt">${esc(n.title)}</div>
            ${n.body ? `<div class="bd">${esc(n.body)}</div>` : ''}
            <div class="meta" title="${esc(fullTime(n.time))}">${esc(ago(n.time))}${n.count > 1 ? ` · ${t('{n}번', { n: n.count })}` : ''}${S.nt.copied === n.id ? ` · <span class="copied">${t('복사했습니다')}</span>` : ''}</div>
          </div>
          <div class="acts">
            <button class="icon-btn small" data-action="notice-toggle-read" data-id="${esc(n.id)}" title="${esc(n.read ? t('안 읽음으로 표시') : t('읽음으로 표시'))}"><span class="unread-dot"></span></button>
            <button class="icon-btn small" data-action="notice-delete" data-id="${esc(n.id)}" title="${esc(t('지우기'))}">${ICONS.x}</button>
          </div>
        </div>`).join('') : `<div class="np-empty">${S.nt.filter === 'unread' ? t('읽지 않은 알림이 없습니다.') : t('알림이 없습니다. 동기화 실패, 결정이 필요한 폴더, 덮어쓰기·옮기기 결과가 여기에 남습니다.')}</div>`}
    </div>`;
}

async function noticeCall(path, body) {
  const r = await api(path, body);
  S.nt.items = r.items || [];
  S.nt.unread = r.unread || 0;
  S.nt.loaded = true;
  renderNoticePanel();
  renderTopbar();
}

function closeNotices() {
  if (!S.nt.open) return;
  S.nt.open = false;
  renderNoticePanel();
  renderTopbar();
}

// 패널 바깥을 누르거나 Esc 면 닫는다 (확인 창이 떠 있을 때는 그대로)
document.addEventListener('mousedown', (e) => {
  if (S.nt.open && !e.target.closest('#notice-panel, [data-action="toggle-notices"], #modal-root')) closeNotices();
});
document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape' && S.nt.open && !$('#modal-root').children.length) closeNotices();
});

// 화면에서 생긴 오류: 알림으로 띄우고 알림 센터에도 남긴다
function noteError(msg) {
  toast(msg, 'err');
  api('/api/notices', { level: 'err', title: msg }).then((r) => {
    S.nt.items = r.items || [];
    S.nt.unread = r.unread || 0;
    renderNoticePanel();
    renderTopbar();
  }).catch(() => {});
}

// 언어를 바꾸면 고정 문구(저장 막대)도 다시 쓴다
function setLang(lang) {
  LANG = lang === 'en' ? 'en' : 'ko';
  document.documentElement.lang = LANG;
  $('#savebar').innerHTML = `
    <span>${t('저장하지 않은 변경 사항이 있습니다.')}</span>
    <div class="savebar-actions">
      <button class="btn ghost" data-action="discard">${t('되돌리기')}</button>
      <button class="btn primary" data-action="save">${t('{store}에 저장', { store: storeWord() })}</button>
    </div>`;
}

function render() {
  renderTopbar();
  renderDrawer();
  renderNoticePanel();
  const main = $('#main');
  if (!S.overview) {
    main.innerHTML = t('<div class="loading">불러오는 중…</div>');
    return;
  }
  // 다시 그려도 입력 중인 칸의 포커스·커서를 지킨다
  const active = document.activeElement;
  const focusSel = active && active.matches('input, textarea, select') ? selectorFor(active) : null;
  const caret = focusSel && 'selectionStart' in active ? [active.selectionStart, active.selectionEnd] : null;
  main.innerHTML = configured() ? renderMain() : renderSetup();
  if (focusSel) {
    const el = $(focusSel, main);
    if (el) {
      el.focus();
      if (caret && el.setSelectionRange) try { el.setSelectionRange(...caret); } catch (e) { /* select 등 */ }
    }
  }
  const log = $('#log');
  if (log) log.scrollTop = log.scrollHeight;
  setTimeout(maybeProbe, 0);
}

// 구글 로그인이 끝나면 드라이브에 GodseonSync 폴더와 setting.json 을 준비한다 (없으면 만든다)
async function runProbe() {
  const su = S.setup;
  if (su.busy) return;
  su.busy = true;
  su.probeError = '';
  render();
  try {
    su.probe = await api('/api/setup/probe', {});
    su.adopt = '';
  } catch (e) {
    su.probe = null;
    su.probeError = e.message;
  } finally {
    su.busy = false;
    render();
  }
}

function maybeProbe() {
  const su = S.setup;
  if (S.overview && !configured() && S.overview.connected && S.overview.connect?.state !== 'running'
    && !su.probe && !su.busy && !su.probeError) runProbe();
}

function selectorFor(el) {
  for (const a of ['data-setup', 'data-global', 'data-bg', 'data-conn']) {
    if (el.hasAttribute(a)) return `[${a}="${el.getAttribute(a)}"]`;
  }
  if (el.dataset.f !== undefined) {
    if (el.dataset.k) return `[data-f="${el.dataset.f}"][data-k="${el.dataset.k}"]`;
    if (el.dataset.p) return `[data-f="${el.dataset.f}"][data-p="${CSS.escape(el.dataset.p)}"]`;
  }
  return null;
}

function updateFolderStatus() {
  if (!configured() || !S.draft) return;
  const alert = $('.tab-alert');
  if (alert) alert.hidden = !S.draft.folders.some((f) => folderState(f).cls === 'err' || folderState(f).choice);
  S.draft.folders.forEach((f, i) => {
    const st = folderState(f);
    const pill = $(`#fstat-${i}`);
    const info = $(`#finfo-${i}`);
    if (pill) {
      pill.className = `pill ${st.cls}`;
      pill.textContent = st.text;
    }
    if (info && info.innerHTML !== st.info) info.innerHTML = st.info;
  });
}

async function refreshLog() {
  const folder = S.logFolder;
  try {
    const r = await api(`/api/log?lines=400${folder ? `&folder=${encodeURIComponent(folder)}` : ''}`);
    if (folder !== S.logFolder) return; // 그새 다른 로그를 골랐다
    const changed = (r.folders || []).join() !== S.logFolders.join();
    S.logText = r.text;
    S.logFolders = r.folders || [];
    const log = $('#log');
    if (log && changed && S.tab === 'settings' && S.settingsTab === 'log') {
      render(); // 고를 수 있는 폴더가 바뀌었다
    } else if (log) {
      const atBottom = log.scrollHeight - log.scrollTop - log.clientHeight < 40;
      log.innerHTML = logHtml(r.text);
      if (atBottom) log.scrollTop = log.scrollHeight;
    }
  } catch (e) { /* 다음 번에 */ }
}

// logChoices 는 설정 › 로그에서 고를 수 있는 폴더다: 이 PC 에서 동기화하는 폴더와, 로그가 남은 폴더.
function logChoices() {
  const folders = S.remote?.config?.folders || S.draft?.folders || [];
  const list = folders.filter((f) => f.id && ((f.paths || {})[me()] || S.logFolders.includes(f.id))).map((f) => ({ id: f.id, name: f.name || f.id }));
  for (const id of S.logFolders) if (!list.some((c) => c.id === id)) list.push({ id, name: id }); // 설정에서 빠진 폴더
  return list;
}

// showLog 는 설정 › 로그에서 그 폴더(빈 값이면 전체) 로그를 연다.
function showLog(id) {
  S.tab = 'settings';
  S.settingsTab = 'log';
  S.logFolder = id;
  savePref('gs-tab', S.tab);
  savePref('gs-settings-tab', S.settingsTab);
  savePref('gs-log-folder', id);
  S.logText = null; // 불러오는 중
  render();
  refreshLog();
}

// logHtml 은 로그에 색을 입힌다. 엔진이 줄 앞에 붙이는 기호(언어와 상관없음)로 나눈다:
// ✗ 실패·오류, ⚠ 확인 필요, ✓ 성공, ▶ 시작, − 지움, … 진행 중. 날짜가 바뀌는 곳에만 날짜를 쓰고 줄마다 시각만 보여 준다.
const LOG_LINE = /^(\d{4}-\d\d-\d\d) (\d\d:\d\d:\d\d) (?:(\[[^\]]*\]) )?(.*)$/;
const LOG_KIND = [['✗', 'err'], ['⚠', 'warn'], ['✓', 'ok'], ['▶', 'start'], ['−', 'del'], ['…', 'muted']];

function logHtml(text) {
  if (text === null) return `<span class="muted">${t('불러오는 중')}…</span>`;
  if (!text) return `<span class="muted">${t('로그가 없습니다.')}</span>`;
  let day = '';
  return text.split('\n').map((line) => {
    const m = LOG_LINE.exec(line);
    if (!m) return esc(line); // 여러 줄 메시지의 뒷줄
    const [, d, time, tag, msg] = m;
    const kind = (LOG_KIND.find(([sym]) => msg.trimStart().startsWith(sym)) || [])[1];
    const head = d !== day ? `<span class="day">${esc(d)}</span>\n` : '';
    day = d;
    const cls = kind && (/^\s/.test(msg) ? `${kind} sub` : kind); // 들여쓴 줄은 rclone 이 낸 세부 줄
    return `${head}<span class="ts">${time}</span> ${tag ? `<span class="tag">${esc(tag)}</span> ` : ''}${cls ? `<span class="${cls}">${esc(msg)}</span>` : esc(msg)}`;
  }).join('\n');
}

// ---------------------------------------------------------------- 입력

document.addEventListener('input', (e) => {
  const el = e.target;
  if (el.dataset.conn !== undefined) {
    S.conn.values[el.dataset.conn] = el.value;
    if (el.tagName === 'SELECT') render(); // 로그인 방식 등: 보일 칸이 바뀐다
    return;
  }
  if (el.dataset.connMode !== undefined) {
    S.conn.mode = el.value;
    render();
    return;
  }
  if (el.dataset.setup) {
    const k = el.dataset.setup;
    S.setup[k] = el.type === 'checkbox' ? el.checked : el.value;
    if (k === 'adopt') render();
    return;
  }
  if (!S.draft) return;
  if (el.dataset.global === 'exclude') {
    S.draft.exclude = el.value.split('\n');
    setDirty(true);
    return;
  }
  if (el.dataset.f === undefined) return;
  const f = S.draft.folders[+el.dataset.f];
  if (!f) return;
  if (el.dataset.p) {
    f.paths = f.paths || {};
    f.paths[el.dataset.p] = el.value;
  } else if (el.dataset.k === 'exclude') {
    f.exclude = el.value.split('\n');
  } else if (el.dataset.k === 'name') {
    // 새 폴더는 이름을 따라 드라이브 경로도 바꿔 준다 (직접 고치기 전까지)
    if (f._autoDrive) {
      f.drive_path = `${S.overview.settings.root}/${el.value.trim()}`;
      const dp = $(`[data-f="${el.dataset.f}"][data-k="drive_path"]`);
      if (dp) dp.value = f.drive_path;
    }
    f.name = el.value;
  } else if (el.dataset.k === 'drive_path') {
    f.drive_path = el.value;
    f._autoDrive = false;
  }
  setDirty(true);
});

document.addEventListener('change', async (e) => {
  const el = e.target;
  if (!el.dataset.bg) return;
  const k = el.dataset.bg;
  const body = {};
  body[k] = el.type === 'checkbox' ? el.checked : k === 'language' ? el.value : +el.value;
  if (k === 'language') {
    try {
      await api('/api/background', body);
      await loadOverview(); // setLang
      render();
      if (S.nt.open) loadNotices();
      toast(t('언어를 바꿨습니다.'));
    } catch (err) {
      noteError(err.message);
    }
    return;
  }
  try {
    await api('/api/background', body);
    await loadOverview();
    render();
    toast(k === 'autostart' ? (el.checked ? t('로그인할 때 자동으로 실행합니다.') : t('자동 실행을 껐습니다.'))
      : k === 'paused' ? (el.checked ? t('자동 동기화를 멈췄습니다.') : t('자동 동기화를 다시 시작합니다.'))
        : t('동기화 주기를 바꿨습니다.'));
  } catch (err) {
    toast(err.message, 'err');
    await loadOverview();
    render();
  }
});

document.addEventListener('toggle', (e) => {
  const d = e.target;
  if (d.dataset && d.dataset.openKey) {
    if (d.open) S.open.add(d.dataset.openKey);
    else S.open.delete(d.dataset.openKey);
  }
}, true);

// ---------------------------------------------------------------- 버튼

const actions = {
  async 'sync-all'() {
    await startSync({});
  },
  async 'sync-folder'(b) {
    await startSync({ folders: [b.dataset.id] });
  },
  async 'resync-folder'(b) {
    if (await confirmBox(t('합치기'), t('이 PC 와 {name}의 파일을 합칩니다. 아무것도 지우지 않고, 양쪽에 같은 파일이 있으면 더 최신 파일을 남깁니다. 지웠던 파일이 다시 살아날 수 있습니다.', { name: storeName() }), t('합치기'))) {
      await startSync({ folders: [b.dataset.id], resync: true });
    }
  },
  async 'dryrun-folder'(b) {
    await startSync({ folders: [b.dataset.id], dry_run: true });
    showLog(b.dataset.id); // 미리보기 결과는 그 폴더 로그에 나온다
  },
  async 'force-folder'(b) {
    if (await confirmBox(t('강제 실행'), t('한 번에 많은 파일이 지워져도 그대로 반영합니다. 다른 PC 와 {store}에서도 지워집니다. 일부러 지운 게 맞을 때만 실행하세요.', { store: storeWord() }), t('강제 실행'), true)) {
      await startSync({ folders: [b.dataset.id], force: true });
    }
  },
  'add-folder'() {
    S.tab = 'folders';
    const root = S.overview.settings.root;
    let n = 1;
    const names = new Set(S.draft.folders.map((f) => f.name));
    while (names.has(`${t('새 폴더')}${n > 1 ? ` ${n}` : ''}`)) n++;
    const name = `${t('새 폴더')}${n > 1 ? ` ${n}` : ''}`;
    S.draft.folders.push({ id: '', name, drive_path: `${root}/${name}`, exclude: [], paths: {}, _autoDrive: true });
    setDirty(true);
    render();
    const i = S.draft.folders.length - 1;
    const input = $(`[data-f="${i}"][data-k="name"]`);
    if (input) {
      input.focus();
      input.select();
      input.scrollIntoView({ block: 'center', behavior: 'smooth' });
    }
  },
  async 'remove-folder'(b) {
    const f = S.draft.folders[+b.dataset.f];
    if (await confirmBox(t('폴더 삭제'), t("'{name}' 을(를) 동기화 목록에서 뺍니다. <b>파일은 지워지지 않습니다</b> (이 PC, 다른 PC, {store} 모두 그대로). 저장해야 반영됩니다.", { name: esc(f.name), store: storeName() }), t('목록에서 빼기'), true)) {
      S.draft.folders.splice(+b.dataset.f, 1);
      setDirty(true);
      render();
    }
  },
  async 'pick-local'(b) {
    const i = +b.dataset.f;
    const f = S.draft.folders[i];
    b.disabled = true;
    try {
      const r = await pickFolder((f.paths || {})[me()] || S.overview.home);
      if (r.path) {
        f.paths = f.paths || {};
        f.paths[me()] = r.path;
        const input = $(`[data-f="${i}"][data-p="${CSS.escape(me())}"]`);
        if (input) input.value = r.path;
        setDirty(true);
      }
    } catch (e) {
      toast(e.message, 'err');
    } finally {
      b.disabled = false;
    }
  },
  async 'browse-drive'(b) {
    const i = +b.dataset.target.split('-')[1];
    const picked = await browseDrive(S.draft.folders[i].drive_path || '');
    if (picked === null) return;
    S.draft.folders[i].drive_path = picked;
    S.draft.folders[i]._autoDrive = false;
    const input = $(`[data-f="${i}"][data-k="drive_path"]`);
    if (input) input.value = picked;
    setDirty(true);
  },
  async save() {
    await saveDraft();
  },
  async discard() {
    if (await confirmBox(t('되돌리기'), t('저장하지 않은 변경 사항을 버리고 {store}의 설정으로 되돌립니다.', { store: storeWord() }), t('되돌리기'))) {
      S.draft = clone(S.remote.config);
      S.draftBase = clone(S.remote.config);
      setDirty(false);
      render();
    }
  },
  async reload() {
    await loadRemote({ keepDraft: true });
    render();
  },
  async 'sync-settings'() {
    await syncSettings({ manual: true });
  },
  'toggle-folder'(b) {
    const f = S.draft.folders[+b.dataset.f];
    if (!f) return;
    if (f.id) {
      if (S.expanded.has(f.id)) S.expanded.delete(f.id);
      else S.expanded.add(f.id);
      saveExpanded();
    } else {
      f._collapsed = !f._collapsed;
    }
    render();
  },
  'toggle-all'() {
    const open = !S.draft.folders.some(isExpanded);
    for (const f of S.draft.folders) {
      if (f.id) {
        if (open) S.expanded.add(f.id);
        else S.expanded.delete(f.id);
      } else {
        f._collapsed = !open;
      }
    }
    saveExpanded();
    render();
  },
  async 'clear-log'() {
    const cur = logChoices().find((c) => c.id === S.logFolder);
    const text = cur
      ? t("'{name}' 폴더의 로그를 지웁니다. 전체 로그와 동기화 설정·파일에는 영향이 없습니다.", { name: esc(cur.name) })
      : t('이 PC 의 GodseonSync 로그를 폴더별 로그까지 모두 지웁니다. 동기화 설정과 파일에는 영향이 없습니다.');
    if (!(await confirmBox(t('로그 지우기'), text, t('지우기'), true))) return;
    const r = await api('/api/log/clear', cur ? { folder: cur.id } : {});
    S.logText = r.text;
    const log = $('#log');
    if (log) log.innerHTML = logHtml(r.text);
    toast(t('로그를 지웠습니다.'));
    refreshLog();
  },
  'log-folder'(b) {
    showLog(b.dataset.id);
  },
  'folder-log'(b) {
    showLog(b.dataset.id);
  },
  async 'refresh-log'() {
    await refreshLog();
  },
  async 'rename-me'() {
    const { action, form } = await modal({
      title: t('이 PC 이름 바꾸기'),
      body: `<label class="field"><span>${t('이름')}</span><input name="name" value="${esc(S.overview.settings.client_name)}"></label>`,
      actions: [{ label: t('취소'), value: null }, { label: t('바꾸기'), kind: 'primary', value: 'ok' }],
    });
    if (action !== 'ok' || !form.name.trim()) return;
    await api('/api/client/rename', { name: form.name.trim() });
    await loadOverview();
    await loadRemote({ keepDraft: true });
    render();
    toast(t('이름을 바꿨습니다.'));
  },
  'toggle-drawer'() {
    S.drawerCollapsed = !S.drawerCollapsed;
    savePref('gs-drawer', S.drawerCollapsed ? 'collapsed' : 'expanded');
    renderDrawer();
  },
  'settings-tab'(b) {
    S.settingsTab = b.dataset.tab;
    savePref('gs-settings-tab', S.settingsTab);
    render();
    if (S.settingsTab === 'log') refreshLog();
  },
  async 'toggle-notices'() {
    S.nt.open = !S.nt.open;
    renderTopbar();
    renderNoticePanel();
    if (S.nt.open) await loadNotices();
  },
  async 'notices-reload'() {
    S.nt.failed = false;
    renderNoticePanel();
    await loadNotices();
  },
  'notices-filter'(b) {
    S.nt.filter = b.dataset.filter;
    renderNoticePanel();
  },
  async 'notice-open'(b) {
    const id = b.dataset.id;
    const sel = getSelection();
    if (sel && !sel.isCollapsed && b.contains(sel.anchorNode)) return; // 글자를 끌어서 일부만 골랐으면 그대로 둔다 (⌘C·Ctrl+C 로 복사)
    const n = S.nt.items.find((x) => x.id === id);
    if (n) { // 누르면 제목·내용·시각을 복사하고, 그 항목 시각 옆에 잠깐 '복사했습니다' (토스트는 패널 머리 버튼을 가린다)
      copyText([n.title, n.body, fullTime(n.time)].filter(Boolean).join('\n')).then(() => {
        S.nt.copied = id;
        renderNoticePanel();
        clearTimeout(S.nt.copiedTimer);
        S.nt.copiedTimer = setTimeout(() => {
          S.nt.copied = null;
          if (getSelection().isCollapsed) renderNoticePanel(); // 그새 글자를 고르고 있으면 다시 그리지 않는다
        }, 1500);
      }, () => toast(t('복사하지 못했습니다.'), 'err'));
    }
    if (S.nt.opened.has(id)) S.nt.opened.delete(id);
    else S.nt.opened.add(id);
    if (n && !n.read) await noticeCall('/api/notices/read', { ids: [id], read: true });
    else renderNoticePanel();
  },
  async 'notice-toggle-read'(b) {
    const n = S.nt.items.find((x) => x.id === b.dataset.id);
    if (n) await noticeCall('/api/notices/read', { ids: [n.id], read: !n.read });
  },
  async 'notice-delete'(b) {
    await noticeCall('/api/notices/delete', { ids: [b.dataset.id] });
  },
  async 'notices-read-all'() {
    await noticeCall('/api/notices/read', { all: true, read: true });
  },
  async 'notices-delete-all'() {
    if (!(await confirmBox(t('알림 모두 지우기'), t('알림 {n}개를 모두 지웁니다.', { n: S.nt.items.length }), t('모두 지우기'), true))) return;
    await noticeCall('/api/notices/delete', { all: true });
  },
  tab(b) {
    S.tab = b.dataset.tab;
    try {
      localStorage.setItem('gs-tab', S.tab);
    } catch (e) { /* 기억 못 해도 이번 화면에서는 바뀐다 */ }
    render();
    if (S.tab === 'settings' && S.settingsTab === 'log') refreshLog();
  },
  async 'remove-client'(b) {
    if (S.dirty) {
      toast(t('먼저 변경 사항을 저장하거나 되돌리세요.'), 'err');
      return;
    }
    if (!(await confirmBox(t('PC 목록에서 빼기'), t("'{name}' 을(를) 목록에서 빼고 그 PC 의 경로 설정을 지웁니다. 그 PC 의 파일은 지워지지 않습니다. 그 PC 에서 GodseonSync 가 다시 실행되면 새 PC 로 다시 나타납니다.", { name: esc(b.dataset.name) }), t('빼기'), true))) return;
    await api('/api/client/remove', { id: b.dataset.id });
    await loadRemote();
    render();
    toast(t('목록에서 뺐습니다.'));
  },
  async connect() {
    const su = S.setup;
    su.probe = null;
    su.probeError = '';
    await api('/api/connect', connectBody());
    await loadOverview();
    S.overview.connected = false;
    render();
  },
  async reconnect() {
    const body = connectBody();
    const conn = S.overview.connection;
    if (conn?.mode === body.mode && conn.info?.scope) body.options.scope = conn.info.scope; // 권한 범위는 지금 것 그대로
    if (body.mode === 'gdrive' && !body.options.client_id && !S.overview.custom_client
      && !(await confirmBox(t('공용 클라이언트로 다시 연결'), t('클라이언트 ID 를 비워 두면 rclone 공용 클라이언트로 다시 로그인합니다. 계속할까요?'), t('다시 연결')))) return;
    S.conn.migrating = false;
    S.conn.moving = false;
    await api('/api/connect', body);
    await loadOverview();
    render();
  },
  async 'choose-folder'(b) {
    const f = S.draft?.folders.find((x) => x.id === b.dataset.id);
    if (!f) return;
    if (S.dirty) {
      toast(t('먼저 변경 사항을 {store}에 저장하거나 되돌리세요.', { store: storeWord() }), 'err');
      return;
    }
    const local = (f.paths || {})[me()] || '';
    const stamp = t('년월일시분초');
    const texts = {
      drive: [t('{store} → 이 PC 덮어쓰기', { store: storeWord() }), t('{store}의 {drive} 내용으로 이 PC 의 {local} 를 덮어씁니다.<br><br>덮어쓰기 전에 이 PC 폴더를 통째로 {backup} 로 복사해 둡니다. {store}에 없는 이 PC 파일은 백업에만 남습니다.',
        { store: storeWord(), drive: `<code>${esc(f.drive_path)}</code>`, local: `<code>${esc(local)}</code>`, backup: `<code>${esc(local)}_backup_${stamp}</code>` }), t('백업하고 덮어쓰기')],
      local: [t('이 PC → {store} 덮어쓰기', { store: storeWord() }), t('이 PC 의 {local} 내용으로 {store}의 {drive} 를 덮어씁니다.<br><br>덮어쓰기 전에 {store} 폴더를 통째로 {backup} 로 복사해 둡니다. 이 PC 에 없는 {store} 파일은 백업에만 남고, 다른 PC 에서도 다음 동기화 때 지워집니다.',
        { store: storeWord(), drive: `<code>${esc(f.drive_path)}</code>`, local: `<code>${esc(local)}</code>`, backup: `<code>${esc(f.drive_path)}_backup_${stamp}</code>` }), t('백업하고 덮어쓰기')],
      merge: [t('합치기'), t('양쪽 파일을 모두 남기고, 같은 파일은 더 최신 것을 남깁니다. 아무것도 지우지 않습니다 (백업 없음).'), t('합치기')],
    };
    const [title, text, ok] = texts[b.dataset.choice];
    if (!(await confirmBox(title, text, ok, b.dataset.choice !== 'merge'))) return;
    const r = await api('/api/folder/choose', { id: f.id, choice: b.dataset.choice });
    toast(r.started ? t('{what}를 시작했습니다.', { what: title }) : `${t('저장했습니다.')} ${r.message || t('다음 동기화 때 진행합니다.')}`);
    setTimeout(pollStatus, 400);
  },
  'pick-mode'(b) {
    S.conn.mode = b.dataset.mode;
    render();
  },
  async 'move-storage'() {
    if (S.dirty) {
      toast(t('먼저 변경 사항을 {store}에 저장하거나 되돌리세요.', { store: storeWord() }), 'err');
      return;
    }
    const target = modeList().find((m) => m.key === connMode())?.label || '';
    if (!(await confirmBox(t('{name}(으)로 바꾸기', { name: esc(target) }), t('이 PC 만 새 저장소로 바꿉니다. 지금 저장소의 파일·설정과 연결(로그인)은 그대로 두어, 설정 › 모드에서 로그인 없이 돌아올 수 있습니다.<br><br>새 저장소에 이 PC 가 든 GodseonSync 설정이 있으면 그 설정을 쓰고, 없으면 지금 설정을 복사해 만듭니다. 파일은 다음 동기화 때 폴더마다 한 번 합치며 올라갑니다 (양쪽에 파일이 있으면 어느 쪽으로 맞출지 묻습니다). 다른 PC 는 각자 모드를 바꿔야 같은 저장소를 씁니다.'), t('바꾸기')))) return;
    const body = connectBody();
    body.move = true;
    S.conn.moving = true;
    S.conn.migrating = false;
    try {
      await api('/api/connect', body);
    } catch (e) {
      S.conn.moving = false;
      throw e;
    }
    await loadOverview();
    render();
  },
  async 'restore-storage'() {
    if (S.dirty) {
      toast(t('먼저 변경 사항을 {store}에 저장하거나 되돌리세요.', { store: storeWord() }), 'err');
      return;
    }
    const saved = S.overview.saved?.[connMode()];
    if (!saved) return;
    if (!(await confirmBox(t('저장된 연결로 바꾸기'), t('저장해 둔 {name} 연결({where})로 바꿉니다. 다시 로그인하지 않고, 그 저장소의 마지막 설정을 그대로 씁니다. 지금 연결은 저장해 두어 다시 돌아올 수 있습니다.', { name: esc(saved.label), where: `<code>${esc(saved.where)}</code>` }), t('바꾸기')))) return;
    S.conn.moving = true;
    S.conn.migrating = false;
    try {
      await api('/api/connect', { mode: connMode(), restore: true });
    } catch (e) {
      S.conn.moving = false;
      throw e;
    }
    await loadOverview();
    render();
  },
  async 'migrate-scope'() {
    if (S.dirty) {
      toast(t('먼저 변경 사항을 {store}에 저장하거나 되돌리세요.', { store: storeWord() }), 'err');
      return;
    }
    const ok = await confirmBox(t("'이 앱이 만든 파일만'으로 옮기기"), t("구글 로그인을 한 번 더 합니다 (같은 계정으로). 로그인이 끝나면:<br>1. 드라이브의 지금 <code>GodseonSync</code> 폴더는 지우지 않고 <b>'GodseonSync (옮기기 전 날짜)'</b> 로 이름만 바꿔 남깁니다.<br>2. 새 <code>GodseonSync</code> 폴더에 지금 설정을 만들고, 이 PC 의 폴더들을 한 번 합치기(지우지 않음)로 다시 올립니다.<br>3. 다른 PC 도 다음 동기화 때 한 번 합칩니다. 다른 PC 는 새 버전으로 업데이트한 뒤 같은 버튼으로 옮기세요.<br><br>옮긴 뒤에는 드라이브 웹에서 GodseonSync 폴더에 직접 넣은 파일은 보이지 않습니다. 옛 폴더는 확인한 뒤 드라이브에서 지우세요."), t('로그인하고 옮기기'));
    if (!ok) return;
    const body = connectBody();
    body.migrate = true;
    S.conn.migrating = true;
    try {
      await api('/api/connect', body);
    } catch (e) {
      S.conn.migrating = false;
      throw e;
    }
    await loadOverview();
    render();
  },
  async 'connect-open'() {
    await api('/api/connect/open', {});
  },
  async 'connect-cancel'() {
    await api('/api/connect/cancel', {});
    await loadOverview();
    render();
  },
  async probe() {
    await runProbe();
  },
  async 'finish-setup'() {
    const su = S.setup;
    su.busy = true;
    render();
    try {
      const r = await api('/api/setup/finish', { client_name: su.name, adopt_client_id: su.adopt, autostart: su.autostart });
      if (r.message) toast(r.message, 'err');
      await loadOverview();
      await loadRemote();
      render();
      toast(t('설정을 마쳤습니다. 이제 동기화할 폴더를 추가하세요.'));
    } catch (e) {
      toast(e.message, 'err');
    } finally {
      su.busy = false;
      render();
    }
  },
  async reset() {
    if (!(await confirmBox(t('처음 설정 다시 하기'), t('이 PC 를 처음 설정 화면으로 돌립니다 (PC 이름, 새 PC 등록·이어받기를 다시 고릅니다). 구글 계정 연결과 파일은 그대로이고, 백그라운드 동기화는 다시 설정할 때까지 멈춥니다.'), t('처음 설정으로')))) return;
    await api('/api/reset', {});
    S.setup.probe = null;
    S.setup.probeError = '';
    await loadOverview();
    render();
  },
  async disconnect() {
    if (!(await confirmBox(t('구글 계정 연결 해제'), t('이 PC 에 저장된 구글 로그인 정보를 지우고 처음 설정 화면으로 돌아갑니다. 파일과 드라이브의 설정은 그대로입니다.'), t('연결 해제'), true))) return;
    await api('/api/disconnect', {});
    S.setup.probe = null;
    S.setup.probeError = '';
    await loadOverview();
    render();
  },
  async quit() {
    if (!(await confirmBox(t('GodseonSync 종료'), t('백그라운드 동기화도 멈춥니다. 다시 켜려면 GodseonSync 를 실행하세요.'), t('종료'), true))) return;
    await api('/api/quit', {});
    $('#main').innerHTML = t('<div class="empty">GodseonSync 를 종료했습니다. 이 창은 닫아도 됩니다.</div>');
    $('#topbar-right').innerHTML = '';
    clearInterval(pollTimer);
  },
};

document.addEventListener('click', async (e) => {
  const b = e.target.closest('[data-action]');
  if (!b || b.disabled) return;
  if (b.dataset.action === 'toggle-folder' && e.target.closest('input, textarea, select') ) return;
  e.preventDefault();
  const fn = actions[b.dataset.action];
  if (!fn) return;
  try {
    await fn(b);
  } catch (err) {
    noteError(err.message);
  }
});

async function startSync(body) {
  if (S.dirty) {
    toast(t('먼저 변경 사항을 {store}에 저장하세요.', { store: storeWord() }), 'err');
    return;
  }
  await api('/api/sync', body);
  toast(body.dry_run ? t('미리보기를 시작했습니다.') : t('동기화를 시작했습니다.'));
  setTimeout(pollStatus, 400);
}

function cleanDraft() {
  const c = clone(S.draft);
  c.exclude = (c.exclude || []).map((s) => s.trim()).filter(Boolean);
  for (const f of c.folders) {
    delete f._autoDrive;
    delete f._collapsed;
    f.name = (f.name || '').trim();
    f.drive_path = (f.drive_path || '').trim();
    f.exclude = (f.exclude || []).map((s) => s.trim()).filter(Boolean);
    for (const [k, v] of Object.entries(f.paths || {})) {
      if (!v || !v.trim()) delete f.paths[k];
      else f.paths[k] = v.trim();
    }
  }
  return c;
}

async function saveDraft() {
  const btn = $('[data-action="save"]');
  btn.disabled = true;
  try {
    const c = cleanDraft();
    const openNew = S.draft.folders.map((f) => !f.id && !f._collapsed);
    // 기준은 편집을 시작한 시점의 설정이다. 그사이 받은 최신 설정(S.remote.config)을 보내면
    // 다른 PC 가 바꾼 폴더 목록을 충돌 없이 덮어써 버린다.
    const r = await api('/api/config/save', { folders: c.folders, exclude: c.exclude, base: S.draftBase || S.remote.config });
    r.config.folders.forEach((f, i) => { if (openNew[i]) S.expanded.add(f.id); });
    saveExpanded();
    S.remote.config = r.config;
    S.remote.cached = false;
    S.remote.warnings = r.warnings;
    S.draft = clone(r.config);
    S.draftBase = clone(r.config);
    S.settingsSync = { state: 'ok', at: new Date().toISOString(), error: '' };
    setDirty(false);
    render();
    toast(t('{store}에 저장했습니다. 다른 PC 는 다음 동기화 때 바뀐 설정을 받습니다.', { store: storeWord() }));
    try {
      await api('/api/sync', {});
      setTimeout(pollStatus, 400);
    } catch (e) { /* 이미 동기화 중 */ }
  } catch (e) {
    if (e.status === 422 && e.data?.errors) {
      await modal({
        title: t('저장하지 못했습니다'),
        body: `<ul style="margin:0;padding-left:18px">${e.data.errors.map((x) => `<li>${esc(x)}</li>`).join('')}</ul>`,
      });
    } else if (e.status === 409) {
      const ok = await confirmBox(t('다른 PC 에서 설정이 바뀌었습니다'), t('다른 PC 가 먼저 설정을 저장했습니다. {store}의 최신 설정을 불러올까요? 지금 고친 내용은 사라지니, 필요하면 다시 입력하세요.', { store: storeWord() }), t('최신 설정 불러오기'));
      if (ok) {
        await loadRemote();
        render();
      }
    } else {
      toast(e.message, 'err');
    }
  } finally {
    btn.disabled = false;
  }
}

// browseDrive 는 드라이브 폴더 고르기 창을 띄우고 고른 경로(취소하면 null)를 돌려준다.
async function browseDrive(start) {
  let path = (start || '').replace(/^\/+|\/+$/g, '');
  const result = modal({
    title: t('{name} 폴더 고르기', { name: storeName() }),
    body: `
      <div class="crumbs" id="dv-crumbs"></div>
      <div class="dir-list" id="dv-list"><div class="none">${t('불러오는 중…')}</div></div>
      <label class="field"><span>${t('고를 경로')}</span><input class="mono" name="path" id="dv-path"></label>
      <p class="field-hint">${t('없는 폴더 이름을 입력하면 첫 동기화 때 만들어집니다.')}</p>`,
    actions: [{ label: t('취소'), value: null }, { label: t('이 폴더 고르기'), kind: 'primary', value: 'ok' }],
    onOpen: (root) => {
      const show = async (p) => {
        path = p;
        $('#dv-path', root).value = p;
        const parts = p ? p.split('/') : [];
        $('#dv-crumbs', root).innerHTML = `<button type="button" data-p="">${storeRoot()}</button>` +
          parts.map((part, i) => `<span>/</span><button type="button" data-p="${esc(parts.slice(0, i + 1).join('/'))}">${esc(part)}</button>`).join('');
        const list = $('#dv-list', root);
        list.innerHTML = t('<div class="none">불러오는 중…</div>');
        try {
          const r = await api(`/api/drive/ls?path=${encodeURIComponent(p)}`);
          if (path !== p) return;
          list.innerHTML = !r.exists ? t('<div class="none">아직 없는 폴더입니다 (첫 동기화 때 만들어집니다).</div>')
            : r.dirs.length ? r.dirs.map((d) => `<button type="button" data-p="${esc(p ? `${p}/${d}` : d)}">📁 ${esc(d)}</button>`).join('')
              : t('<div class="none">하위 폴더가 없습니다.</div>');
        } catch (e) {
          list.innerHTML = `<div class="none">${esc(e.message)}</div>`;
        }
      };
      root.addEventListener('click', (e) => {
        const b = e.target.closest('button[data-p]');
        if (b) show(b.dataset.p);
      });
      show(path);
    },
  });
  const { action, form } = await result;
  if (action !== 'ok') return null;
  return (form.path || '').trim().replace(/^\/+|\/+$/g, '');
}

window.addEventListener('beforeunload', (e) => {
  if (S.dirty) {
    e.preventDefault();
    e.returnValue = '';
  }
});

// ---------------------------------------------------------------- 시작

let pollTimer;

async function init() {
  if (!TOKEN) {
    $('#main').innerHTML = t('<div class="empty">주소에 접속 토큰이 없습니다. 트레이(메뉴 막대) 아이콘의 <b>설정 열기</b>로 여세요.</div>');
    return;
  }
  try {
    await loadOverview();
  } catch (e) {
    $('#main').innerHTML = `<div class="empty">${esc(e.message)}</div>`;
    return;
  }
  await pollStatus();
  if (configured() && (await loadCached())) {
    // 이 PC 에 저장해 둔 마지막 설정으로 바로 열고, 드라이브의 최신 설정은 뒤에서 받는다
    render();
    syncSettings();
  } else {
    // 캐시가 없으면(처음 설정 직후) 드라이브에서 받을 때까지 기다린다
    if (configured()) await loadRemote();
    render();
  }
  refreshLog();
  pollTimer = setInterval(pollStatus, 3000);
  // 다른 PC 의 설정·상태는 1분마다 새로 받는다 (편집 중인 내용은 지킨다)
  setInterval(() => {
    if (!document.hidden && !S.status?.running) syncSettings();
  }, 60000);
}

init();
