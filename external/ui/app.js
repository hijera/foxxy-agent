const HDR = 'X-Coddy-Session-ID';

function randomSessionId() {
  const hex = [...crypto.getRandomValues(new Uint8Array(18))].map((b) => b.toString(16).padStart(2, '0')).join('');
  return `sess_${hex}`;
}

function getSessionFromHash() {
  const h = window.location.hash.replace(/^#\/?/, '');
  const m = /^s\/([^/]+)$/.exec(h);
  return m ? decodeURIComponent(m[1]) : '';
}

function setSessionHash(id) {
  const next = `#/s/${encodeURIComponent(id)}`;
  if (window.location.hash !== next) {
    history.replaceState(null, '', `${window.location.pathname}${window.location.search}${next}`);
  }
}

const state = {
  sessionId: '',
  sessionCursor: null,
  hasMoreSessions: false,
  memSel: { root: '', path: '', rel: '' },
  streamTools: '',
};

async function api(path, opts = {}) {
  const headers = Object.assign({}, opts.headers || {});
  headers['X-Coddy-Session-ID'] = state.sessionId;
  return fetch(path, Object.assign({}, opts, { headers }));
}

function headersOnly() {
  return { [HDR]: state.sessionId };
}

function syncSessionBootstrap() {
  let id = getSessionFromHash();
  try {
    if (!id) {
      id = randomSessionId();
      setSessionHash(id);
    }
    state.sessionId = id;
  } catch {
    state.sessionId = randomSessionId();
    setSessionHash(state.sessionId);
  }
}

function el(id) {
  return document.getElementById(id);
}

function renderTodos(entries) {
  const box = el('todo-list');
  box.innerHTML = '';
  (entries || []).forEach((e, i) => {
    const row = document.createElement('div');
    row.className = 'todo-entry';
    const inp = document.createElement('textarea');
    inp.rows = 2;
    inp.value = e.content || '';
    const status = document.createElement('select');
    ['pending', 'in_progress', 'completed', 'failed'].forEach((s) => {
      const o = document.createElement('option');
      o.value = s;
      o.textContent = s;
      if ((e.status || 'pending') === s) {
        o.selected = true;
      }
      status.appendChild(o);
    });
    inp.dataset.idx = String(i);
    status.dataset.idx = String(i);
    row.appendChild(inp);
    row.appendChild(status);
    box.appendChild(row);
  });
  state.todoDraft = entries || [];
}

function collectTodosFromDOM() {
  const box = el('todo-list');
  const rows = [...box.querySelectorAll('.todo-entry')];
  return rows.map((row) => {
    const inp = row.querySelector('textarea');
    const sel = row.querySelector('select');
    return {
      content: inp?.value?.trim() || '',
      status: sel?.value || 'pending',
    };
  });
}

async function loadPlan() {
  const res = await api(`/coddy/sessions/${encodeURIComponent(state.sessionId)}/plan`);
  if (!res.ok) {
    return;
  }
  const data = await res.json();
  renderTodos(data.entries || []);
}

async function savePlan() {
  const entries = collectTodosFromDOM();
  const res = await api(`/coddy/sessions/${encodeURIComponent(state.sessionId)}/plan`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ entries }),
  });
  if (!res.ok) {
    alert('Failed to save todos');
  }
}

async function loadMessages() {
  const res = await api(`/coddy/sessions/${encodeURIComponent(state.sessionId)}/messages`);
  if (!res.ok) {
    return;
  }
  const data = await res.json();
  const wrap = el('messages');
  wrap.innerHTML = '';
  (data.messages || []).forEach((m) => {
    if (m.role === 'user' || m.role === 'assistant') {
      appendBubble(m.role, m.content || '');
    }
  });
}

function appendBubble(role, text) {
  const wrap = el('messages');
  const div = document.createElement('div');
  div.className = `msg ${role === 'user' ? 'msg-user' : 'msg-assistant'}`;
  div.textContent = text;
  wrap.appendChild(div);
  wrap.scrollTop = wrap.scrollHeight;
}

function appendToolsLine(line) {
  const wrap = el('messages');
  let block = wrap.querySelector('.msg-tools:last-of-type');
  if (!block || !block.dataset.live) {
    block = document.createElement('div');
    block.className = 'msg-tools';
    block.dataset.live = '1';
    wrap.appendChild(block);
  }
  const p = document.createElement('div');
  p.className = 'tool-line';
  p.textContent = line;
  block.appendChild(p);
  wrap.scrollTop = wrap.scrollHeight;
}

function clearLiveTools() {
  document.querySelectorAll('.msg-tools[data-live]').forEach((n) => n.removeAttribute('data-live'));
}

function setTokenBar(u) {
  const bar = el('token-bar');
  if (!u) {
    bar.hidden = true;
    bar.textContent = '';
    return;
  }
  bar.hidden = false;
  bar.textContent = `Tokens in this turn: input ${u.inputTokens} | output ${u.outputTokens} | total ${u.totalTokens}`;
}

async function loadSessions(init) {
  const ps = new URLSearchParams();
  ps.set('limit', '30');
  if (!init && state.sessionCursor) {
    ps.set('cursor', state.sessionCursor);
  }
  const res = await api(`/coddy/sessions?${ps.toString()}`, { headers: {} });
  if (!res.ok) {
    return;
  }
  const data = await res.json();
  const list = el('session-list');
  if (init) {
    list.innerHTML = '';
  }
  (data.sessions || []).forEach((s) => {
    if (list.querySelector(`[data-id="${CSS.escape(s.id)}"]`)) {
      return;
    }
    const row = document.createElement('div');
    row.className = 'session-item';
    row.dataset.id = s.id;
    if (s.id === state.sessionId) {
      row.classList.add('active');
    }
    row.innerHTML = `<div class="session-row"><span class="session-title"></span><button class="session-menu" type="button">&#8226;&#8226;&#8226;</button></div>`;
    row.querySelector('.session-title').textContent = s.title || s.id;
    row.addEventListener('click', (ev) => {
      if (ev.target.closest('.session-menu')) {
        return;
      }
      pickSession(s.id);
    });
    row.querySelector('.session-menu').addEventListener('click', (ev) => {
      ev.stopPropagation();
      void sessionMenu(s.id, s.title || s.id);
    });
    list.appendChild(row);
  });
  state.sessionCursor = data.nextCursor || null;
  state.hasMoreSessions = !!data.hasMore;
}

async function sessionMenu(id, title) {
  const action = window.prompt(`Session ${id}\nRename: type new title, or type DELETE to remove`, title);
  if (action === null) {
    return;
  }
  const t = action.trim();
  if (t.toUpperCase() === 'DELETE') {
    const res = await fetch(`/coddy/sessions/${encodeURIComponent(id)}`, {
      method: 'DELETE',
      headers: headersOnly(),
    });
    if (res.ok && id === state.sessionId) {
      pickSession(randomSessionId());
    } else if (res.ok) {
      await loadSessions(true);
    }
    return;
  }
  if (t !== '') {
    await fetch(`/coddy/sessions/${encodeURIComponent(id)}`, {
      method: 'PATCH',
      headers: Object.assign(headersOnly(), { 'Content-Type': 'application/json' }),
      body: JSON.stringify({ title: t }),
    });
    await loadSessions(true);
  }
}

function pickSession(id) {
  state.sessionId = id;
  setSessionHash(id);
  document.querySelectorAll('.session-item').forEach((n) => {
    n.classList.toggle('active', n.dataset.id === id);
  });
  el('chat-title').textContent = id;
  setTokenBar(null);
  void loadMessages();
  void loadPlan();
  void refreshMemoryRoots();
}

function parseSSEBlocks(chunk, carry) {
  const text = carry.buf + chunk;
  const parts = text.split(/\n\n+/);
  carry.buf = parts.pop() || '';
  const events = [];
  for (const blk of parts) {
    let evName = '';
    const dataLines = [];
    blk.split('\n').forEach((ln) => {
      if (ln.startsWith('event:')) {
        evName = ln.slice(6).trim();
      }
      if (ln.startsWith('data:')) {
        dataLines.push(ln.slice(5).trim());
      }
    });
    events.push({ event: evName, data: dataLines.join('\n') });
  }
  return events;
}

async function streamResponses(userText) {
  appendBubble('user', userText);
  clearLiveTools();
  setTokenBar(null);
  const assistantEl = document.createElement('div');
  assistantEl.className = 'msg msg-assistant';
  assistantEl.textContent = '';
  el('messages').appendChild(assistantEl);

  const res = await fetch('/v1/responses', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      [HDR]: state.sessionId,
    },
    body: JSON.stringify({ model: 'agent', input: userText, stream: true }),
  });
  const sidHdr = res.headers.get(HDR);
  if (sidHdr && sidHdr !== state.sessionId) {
    state.sessionId = sidHdr;
    setSessionHash(sidHdr);
  }
  if (!res.ok || !res.body) {
    assistantEl.textContent = `Request failed (${res.status})`;
    return;
  }
  const reader = res.body.getReader();
  const dec = new TextDecoder();
  const carry = { buf: '' };
  let done = false;
  while (!done) {
    const step = await reader.read();
    done = step.done;
    if (step.value) {
      const events = parseSSEBlocks(dec.decode(step.value, { stream: true }), carry);
      for (const ev of events) {
        if (ev.data === '[DONE]') {
          continue;
        }
        if (!ev.event) {
          try {
            const delta = JSON.parse(ev.data);
            const piece =
              delta.choices?.[0]?.delta?.content ||
              delta.choices?.[0]?.delta?.reasoning_content ||
              '';
            if (piece) {
              assistantEl.textContent += piece;
              el('messages').scrollTop = el('messages').scrollHeight;
            }
          } catch {
            /* ignore */
          }
        } else if (ev.event === 'token_usage') {
          try {
            const u = JSON.parse(ev.data);
            setTokenBar(u);
          } catch {
            /* ignore */
          }
        } else if (ev.event === 'tool_call') {
          try {
            const t = JSON.parse(ev.data);
            appendToolsLine(`${t.kind || 'tool'}: ${t.title || t.toolCallId || ''} (${t.status || ''})`);
          } catch {
            /* ignore */
          }
        } else if (ev.event === 'tool_call_update') {
          try {
            const t = JSON.parse(ev.data);
            appendToolsLine(`update ${t.toolCallId}: ${t.status}`);
          } catch {
            /* ignore */
          }
        } else if (ev.event === 'plan') {
          try {
            const p = JSON.parse(ev.data);
            renderTodos(p.entries || []);
          } catch {
            /* ignore */
          }
        }
      }
    }
  }
  carry.buf += dec.decode(new Uint8Array(), { stream: false });
  if (carry.buf.trim()) {
    const events = parseSSEBlocks('', carry);
    for (const ev of events.filter((x) => x.data)) {
      /** minimal flush */
    }
  }
  clearLiveTools();
  await loadSessions(true);
}

async function sendChat() {
  const ta = el('composer');
  const txt = ta.value.trim();
  if (!txt) {
    return;
  }
  ta.value = '';
  await streamResponses(txt);
}

async function refreshMemoryRoots() {
  state.memSel = { root: '', path: '', rel: '' };
  el('mem-tree').textContent = 'Pick global or workspace';
  el('mem-editor').value = '';
}

async function loadMemoryTree(root, path) {
  state.memSel.root = root;
  state.memSel.rel = path || '';
  const ps = new URLSearchParams({ root });
  if (path) {
    ps.set('path', path);
  }
  const res = await api(`/coddy/sessions/${encodeURIComponent(state.sessionId)}/memory/tree?${ps}`);
  if (!res.ok) {
    return;
  }
  const data = await res.json();
  const box = el('mem-tree');
  box.innerHTML = '';
  if (data.roots) {
    data.roots.forEach((r) => {
      const d = document.createElement('div');
      d.className = 'mem-node';
      d.textContent = `[${r.id}]`;
      d.addEventListener('click', () => {
        void loadMemoryTree(r.id, '');
      });
      box.appendChild(d);
    });
    return;
  }
  data.nodes.forEach((n) => {
    const d = document.createElement('div');
    d.className = 'mem-node';
    const prefix = n.kind === 'dir' ? 'dir ' : 'file ';
    d.textContent = `${prefix}${n.name}`;
    d.addEventListener('click', () => {
      if (n.kind === 'dir') {
        const next = data.path ? `${data.path}/${n.name}` : n.name;
        void loadMemoryTree(data.root, next);
      } else {
        state.memSel.path = data.path ? `${data.path}/${n.name}` : n.name;
        void openMemoryFile(data.root, state.memSel.path);
      }
    });
    box.appendChild(d);
  });
}

async function openMemoryFile(root, pathRel) {
  const ps = new URLSearchParams({ root, path: pathRel });
  const res = await api(`/coddy/sessions/${encodeURIComponent(state.sessionId)}/memory/file?${ps}`);
  if (!res.ok) {
    return;
  }
  const data = await res.json();
  el('mem-editor').value = data.content || '';
  state.memSel.root = root;
  state.memSel.path = pathRel;
}

async function saveMemoryFile() {
  const { root, path } = state.memSel;
  if (!root || !path) {
    alert('Pick a path first');
    return;
  }
  const body = JSON.stringify({
    root,
    path,
    content: el('mem-editor').value,
  });
  await api(`/coddy/sessions/${encodeURIComponent(state.sessionId)}/memory/file`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body,
  });
}

async function deleteMemoryFile() {
  const { root, path } = state.memSel;
  if (!root || !path) {
    return;
  }
  if (!window.confirm('Delete this file?')) {
    return;
  }
  const ps = new URLSearchParams({ root, path });
  await fetch(`/coddy/sessions/${encodeURIComponent(state.sessionId)}/memory/file?${ps}`, {
    method: 'DELETE',
    headers: headersOnly(),
  });
  el('mem-editor').value = '';
  await loadMemoryTree(root, state.memSel.rel);
}

syncSessionBootstrap();

window.addEventListener('hashchange', () => {
  const id = getSessionFromHash();
  if (id && id !== state.sessionId) {
    pickSession(id);
  }
});

el('btn-send').addEventListener('click', () => void sendChat());
el('composer').addEventListener('keydown', (ev) => {
  if (ev.key === 'Enter' && !ev.shiftKey) {
    ev.preventDefault();
    void sendChat();
  }
});
el('btn-new').addEventListener('click', () => {
  pickSession(randomSessionId());
});
el('btn-load-more').addEventListener('click', () => void loadSessions(false));

el('btn-save-plan').addEventListener('click', () => void savePlan());
el('mem-root-global').addEventListener('click', () => void loadMemoryTree('global', ''));
el('mem-root-workspace').addEventListener('click', () => void loadMemoryTree('workspace', ''));

el('btn-mem-save').addEventListener('click', () => void saveMemoryFile());
el('btn-mem-delete').addEventListener('click', () => void deleteMemoryFile());

pickSession(state.sessionId);
void loadSessions(true);
