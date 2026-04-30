'use strict';

// ── State ──────────────────────────────────────────────────────────────────
const state = {
  playerId: null,
  turn: 1,
  units: [],
  regions: [],
  paths: [],
  unitConfigs: {},
  ringBearer: { currentRegion: '', lastDetectedRegion: '', lastDetectedTurn: 0 },
  selectedUnit: null,
  eventSource: null,
};

const API = window.location.origin;

// ── Region map coordinates (hand-tuned for the SVG viewBox 900×700) ───────
const REGION_POS = {
  'the-shire':     { x: 80,  y: 100 },
  'bree':          { x: 200, y: 120 },
  'tharbad':       { x: 160, y: 230 },
  'weathertop':    { x: 310, y: 110 },
  'rivendell':     { x: 400, y: 80  },
  'fangorn':       { x: 230, y: 340 },
  'fords-of-isen': { x: 180, y: 310 },
  'rohan-plains':  { x: 310, y: 290 },
  'moria':         { x: 430, y: 170 },
  'helms-deep':    { x: 240, y: 400 },
  'isengard':      { x: 200, y: 370 },
  'edoras':        { x: 330, y: 380 },
  'lothlorien':    { x: 450, y: 240 },
  'dead-marshes':  { x: 560, y: 300 },
  'emyn-muil':     { x: 530, y: 240 },
  'minas-tirith':  { x: 440, y: 430 },
  'ithilien':      { x: 580, y: 390 },
  'osgiliath':     { x: 520, y: 450 },
  'minas-morgul':  { x: 600, y: 470 },
  'cirith-ungol':  { x: 680, y: 420 },
  'mordor':        { x: 700, y: 530 },
  'mount-doom':    { x: 790, y: 570 },
};

// Path connections (subset for drawing — matches PathConfig)
const PATH_CONNECTIONS = [
  ['the-shire','bree'],['bree','weathertop'],['bree','rivendell'],['bree','tharbad'],
  ['the-shire','tharbad'],['weathertop','rivendell'],['rivendell','moria'],
  ['rivendell','lothlorien'],['moria','lothlorien'],['lothlorien','emyn-muil'],
  ['lothlorien','rohan-plains'],['rohan-plains','fangorn'],['rohan-plains','edoras'],
  ['rohan-plains','minas-tirith'],['fangorn','isengard'],['isengard','rohan-plains'],
  ['tharbad','fords-of-isen'],['fords-of-isen','isengard'],['fords-of-isen','helms-deep'],
  ['fords-of-isen','edoras'],['edoras','helms-deep'],['helms-deep','isengard'],
  ['edoras','minas-tirith'],['emyn-muil','dead-marshes'],['emyn-muil','ithilien'],
  ['dead-marshes','ithilien'],['dead-marshes','mordor'],['ithilien','minas-tirith'],
  ['ithilien','osgiliath'],['ithilien','cirith-ungol'],['minas-tirith','osgiliath'],
  ['osgiliath','minas-morgul'],['minas-morgul','cirith-ungol'],['minas-morgul','mordor'],
  ['cirith-ungol','mordor'],['cirith-ungol','mount-doom'],['mordor','mount-doom'],
];

const PATH_ID_MAP = {
  'the-shire-bree':           'shire-to-bree',
  'bree-weathertop':          'bree-to-weathertop',
  'bree-rivendell':           'bree-to-rivendell',
  'bree-tharbad':             'bree-to-tharbad',
  'the-shire-tharbad':        'shire-to-tharbad',
  'weathertop-rivendell':     'weathertop-to-rivendell',
  'rivendell-moria':          'rivendell-to-moria',
  'rivendell-lothlorien':     'rivendell-to-lothlorien',
  'moria-lothlorien':         'moria-to-lothlorien',
  'lothlorien-emyn-muil':     'lothlorien-to-emyn-muil',
  'lothlorien-rohan-plains':  'lothlorien-to-rohan-plains',
  'rohan-plains-fangorn':     'rohan-plains-to-fangorn',
  'rohan-plains-edoras':      'rohan-plains-to-edoras',
  'rohan-plains-minas-tirith':'rohan-plains-to-minas-tirith',
  'fangorn-isengard':         'fangorn-to-isengard',
  'isengard-rohan-plains':    'isengard-to-rohan-plains',
  'tharbad-fords-of-isen':    'tharbad-to-fords-of-isen',
  'fords-of-isen-isengard':   'fords-of-isen-to-isengard',
  'fords-of-isen-helms-deep': 'fords-of-isen-to-helms-deep',
  'fords-of-isen-edoras':     'fords-of-isen-to-edoras',
  'edoras-helms-deep':        'edoras-to-helms-deep',
  'helms-deep-isengard':      'helms-deep-to-isengard',
  'edoras-minas-tirith':      'edoras-to-minas-tirith',
  'emyn-muil-dead-marshes':   'emyn-muil-to-dead-marshes',
  'emyn-muil-ithilien':       'emyn-muil-to-ithilien',
  'dead-marshes-ithilien':    'dead-marshes-to-ithilien',
  'dead-marshes-mordor':      'dead-marshes-to-mordor',
  'ithilien-minas-tirith':    'ithilien-to-minas-tirith',
  'ithilien-osgiliath':       'ithilien-to-osgiliath',
  'ithilien-cirith-ungol':    'ithilien-to-cirith-ungol',
  'minas-tirith-osgiliath':   'minas-tirith-to-osgiliath',
  'osgiliath-minas-morgul':   'osgiliath-to-minas-morgul',
  'minas-morgul-cirith-ungol':'minas-morgul-to-cirith-ungol',
  'minas-morgul-mordor':      'minas-morgul-to-mordor',
  'cirith-ungol-mordor':      'cirith-ungol-to-mordor',
  'cirith-ungol-mount-doom':  'cirith-ungol-to-mount-doom',
  'mordor-mount-doom':        'mordor-to-mount-doom',
};

const TERRAIN_COLOR = {
  PLAINS: '#4a7c59', MOUNTAINS: '#7a6e5f', FOREST: '#2d5a27',
  FORTRESS: '#5a4a3a', VOLCANIC: '#8b2500', SWAMP: '#3d4f2e',
};

// ── Entry Point ────────────────────────────────────────────────────────────
function selectPlayer(side) {
  state.playerId = side;
  document.getElementById('player-select').classList.add('hidden');
  document.getElementById('game-main').classList.remove('hidden');

  const label = side === 'light' ? '☀️ Light Side' : '🌑 Dark Side';
  document.getElementById('player-display').textContent = label;
  document.getElementById('player-display').style.color = side === 'light' ? '#4a90d9' : '#cc3333';

  document.getElementById('analysis-title').textContent =
    side === 'light' ? 'Route Risk Analysis' : 'Interception Analysis';

  initMap();
  loadGameState();
  connectSSE();
}

// ── SSE ────────────────────────────────────────────────────────────────────
function connectSSE() {
  if (state.eventSource) state.eventSource.close();
  const url = `${API}/events?playerId=${state.playerId}`;
  state.eventSource = new EventSource(url);

  state.eventSource.onopen = () => {
    document.getElementById('status-display').textContent = 'Connected';
    document.getElementById('status-display').className = 'status-ok';
  };

  state.eventSource.onmessage = (e) => {
    try {
      const event = JSON.parse(e.data);
      handleServerEvent(event);
    } catch (_) {}
  };

  state.eventSource.onerror = () => {
    document.getElementById('status-display').textContent = 'Reconnecting…';
    document.getElementById('status-display').className = 'status-err';
  };
}

function handleServerEvent(event) {
  const topic = event.topic || event.type;

  switch (topic) {
    case 'game.broadcast':
      if (event.payload) applySnapshot(event.payload);
      addLog(`Turn ${event.payload?.turn || '?'} snapshot received`, 'info');
      break;

    case 'game.ring.position':
      if (event.payload?.trueRegion) {
        state.ringBearer.currentRegion = event.payload.trueRegion;
        addLog(`Ring Bearer moved to ${event.payload.trueRegion}`, 'highlight');
        renderUnits();
      }
      break;

    case 'game.ring.detection': {
      const region = event.payload?.regionId || event.payload?.pathId || '?';
      addLog(`⚠ Ring Bearer DETECTED near ${region}!`, 'danger');
      document.body.classList.add('detected');
      setTimeout(() => document.body.classList.remove('detected'), 1800);
      if (event.payload?.regionId) {
        state.ringBearer.lastDetectedRegion = event.payload.regionId;
        state.ringBearer.lastDetectedTurn = event.payload.turn;
      }
      break;
    }

    case 'game.events.unit':
      addLog(`Unit event: ${JSON.stringify(event.payload).slice(0, 80)}`);
      loadGameState();
      break;

    case 'game.events.path':
      addLog(`Path event: ${event.key} → ${event.payload?.status || ''}`);
      loadGameState();
      break;

    case 'game.events.region':
      addLog(`Region event: ${event.key}`);
      loadGameState();
      break;

    default:
      if (event.payload?.winner) {
        showGameOver(event.payload.winner, event.payload.cause);
      }
  }
}

// ── Load Game State ────────────────────────────────────────────────────────
async function loadGameState() {
  try {
    const res = await fetch(`${API}/game/state?playerId=${state.playerId}`);
    if (!res.ok) return;
    const data = await res.json();
    applySnapshot(data);
  } catch (err) {
    console.error('loadGameState:', err);
  }
}

function applySnapshot(data) {
  if (data.turn) {
    state.turn = data.turn;
    document.getElementById('turn-display').textContent = `Turn: ${data.turn}`;
  }
  if (data.units)   state.units   = data.units;
  if (data.regions) state.regions = data.regions;
  if (data.paths)   state.paths   = data.paths;
  if (data.ringBearer) {
    if (state.playerId === 'light' && data.ringBearer.currentRegion) {
      state.ringBearer.currentRegion = data.ringBearer.currentRegion;
    }
    if (data.ringBearer.lastDetectedRegion) {
      state.ringBearer.lastDetectedRegion = data.ringBearer.lastDetectedRegion;
    }
  }

  renderUnits();
  renderMapUnits();
  renderMapPaths();
  populateUnitSelector();
}

// ── Map Rendering ──────────────────────────────────────────────────────────
function initMap() {
  const svg = document.getElementById('map-svg');

  // Draw static region nodes
  const regLayer = document.getElementById('regions-layer');
  regLayer.innerHTML = '';
  for (const [id, pos] of Object.entries(REGION_POS)) {
    const g = document.createElementNS('http://www.w3.org/2000/svg', 'g');
    g.setAttribute('class', 'region-node');
    g.setAttribute('data-region', id);
    g.setAttribute('transform', `translate(${pos.x},${pos.y})`);
    g.addEventListener('click', () => onRegionClick(id));

    const circle = document.createElementNS('http://www.w3.org/2000/svg', 'circle');
    circle.setAttribute('r', '14');
    circle.setAttribute('fill', '#1a2a3a');
    circle.setAttribute('stroke', '#3a4a6a');
    circle.setAttribute('stroke-width', '1.5');

    const label = document.createElementNS('http://www.w3.org/2000/svg', 'text');
    label.setAttribute('class', 'region-label');
    label.setAttribute('text-anchor', 'middle');
    label.setAttribute('y', '24');
    label.textContent = id.replace(/-/g, ' ').replace(/\b\w/g, c => c.toUpperCase()).slice(0, 12);

    g.appendChild(circle);
    g.appendChild(label);
    regLayer.appendChild(g);
  }

  // Draw static path lines
  renderMapPaths();
}

function renderMapPaths() {
  const pathLayer = document.getElementById('paths-layer');
  pathLayer.innerHTML = '';

  const pathMap = {};
  for (const p of state.paths) pathMap[p.id] = p;

  for (const [from, to] of PATH_CONNECTIONS) {
    const posA = REGION_POS[from];
    const posB = REGION_POS[to];
    if (!posA || !posB) continue;

    const key = `${from}-${to}`;
    const pathId = PATH_ID_MAP[key] || PATH_ID_MAP[`${to}-${from}`];
    const pathState = pathId ? pathMap[pathId] : null;
    const status = pathState?.status || 'OPEN';

    const line = document.createElementNS('http://www.w3.org/2000/svg', 'line');
    line.setAttribute('x1', posA.x); line.setAttribute('y1', posA.y);
    line.setAttribute('x2', posB.x); line.setAttribute('y2', posB.y);
    line.setAttribute('class', `path-line path-${status.toLowerCase().replace('_', '-')}`);
    if (pathId) line.setAttribute('data-path', pathId);
    line.addEventListener('click', () => {
      if (pathId) {
        document.getElementById('inp-target-path').value = pathId;
        document.getElementById('inp-path-ids').value = pathId;
      }
    });
    pathLayer.appendChild(line);
  }

  // Update region colors by control
  const regionMap = {};
  for (const r of state.regions) regionMap[r.id] = r;

  for (const node of document.querySelectorAll('.region-node')) {
    const rid = node.getAttribute('data-region');
    const r = regionMap[rid];
    const circle = node.querySelector('circle');
    if (!circle) continue;
    if (r?.controlledBy === 'FREE_PEOPLES') {
      circle.setAttribute('stroke', '#4a90d9');
      circle.setAttribute('fill', '#1a2a4a');
    } else if (r?.controlledBy === 'SHADOW') {
      circle.setAttribute('stroke', '#cc3333');
      circle.setAttribute('fill', '#2a1010');
    } else {
      circle.setAttribute('stroke', '#666');
      circle.setAttribute('fill', '#1a2a3a');
    }
    if (r?.fortified) circle.setAttribute('stroke-width', '3');
    // Special marks
    if (rid === 'mount-doom') circle.setAttribute('fill', '#5a1000');
    if (rid === 'the-shire') circle.setAttribute('fill', '#1a3a2a');
  }
}

function renderMapUnits() {
  const unitLayer = document.getElementById('units-layer');
  unitLayer.innerHTML = '';

  // Group units by region
  const byRegion = {};
  for (const u of state.units) {
    if (!u.region || u.status !== 'ACTIVE') continue;
    if (!byRegion[u.region]) byRegion[u.region] = [];
    byRegion[u.region].push(u);
  }

  // Ring bearer (light side only)
  if (state.playerId === 'light' && state.ringBearer.currentRegion) {
    const rbRegion = state.ringBearer.currentRegion;
    if (!byRegion[rbRegion]) byRegion[rbRegion] = [];
    byRegion[rbRegion].push({ id: 'ring-bearer', side: 'FREE_PEOPLES', symbol: '💍' });
  }

  for (const [region, units] of Object.entries(byRegion)) {
    const pos = REGION_POS[region];
    if (!pos) continue;
    units.forEach((u, i) => {
      const g = document.createElementNS('http://www.w3.org/2000/svg', 'g');
      g.setAttribute('class', 'unit-marker');
      const offsetX = (i % 3) * 12 - 12;
      const offsetY = Math.floor(i / 3) * -12;
      g.setAttribute('transform', `translate(${pos.x + offsetX},${pos.y + offsetY})`);

      const dot = document.createElementNS('http://www.w3.org/2000/svg', 'circle');
      dot.setAttribute('r', '5');
      dot.setAttribute('fill', u.side === 'FREE_PEOPLES' || u.side === 'light' ? '#4a90d9' : '#cc3333');
      dot.setAttribute('stroke', '#fff');
      dot.setAttribute('stroke-width', '0.5');

      g.appendChild(dot);
      g.addEventListener('click', () => selectUnit(u.id));
      unitLayer.appendChild(g);
    });
  }

  // Last detected ring bearer marker (dark side)
  if (state.playerId === 'dark' && state.ringBearer.lastDetectedRegion) {
    const pos = REGION_POS[state.ringBearer.lastDetectedRegion];
    if (pos) {
      const g = document.createElementNS('http://www.w3.org/2000/svg', 'g');
      g.setAttribute('transform', `translate(${pos.x},${pos.y})`);
      const eye = document.createElementNS('http://www.w3.org/2000/svg', 'text');
      eye.setAttribute('text-anchor', 'middle');
      eye.setAttribute('y', '-18');
      eye.setAttribute('font-size', '14');
      eye.textContent = '👁';
      g.appendChild(eye);
      document.getElementById('units-layer').appendChild(g);
    }
  }
}

// ── Units Panel ────────────────────────────────────────────────────────────
function renderUnits() {
  const list = document.getElementById('units-list');
  list.innerHTML = '';

  const mySide = state.playerId === 'light' ? 'FREE_PEOPLES' : 'SHADOW';
  const myUnits = state.units.filter(u => {
    // Infer side from known unit IDs (no hardcoding in game logic, but UI can map names)
    return true; // Show all, highlight own side
  });

  for (const u of state.units) {
    const card = document.createElement('div');
    card.className = `unit-card ${u.status?.toLowerCase() || 'active'}`;
    card.setAttribute('data-unit', u.id);
    card.addEventListener('click', () => selectUnit(u.id));

    let regionDisplay = u.region || '—';
    if (u.id === 'ring-bearer') {
      regionDisplay = state.playerId === 'light'
        ? (state.ringBearer.currentRegion || '?')
        : '??? (hidden)';
    }

    card.innerHTML = `
      <div>
        <div class="unit-name">${u.id}</div>
        <div class="unit-region">${regionDisplay}</div>
      </div>
      <div class="unit-str">⚔ ${u.strength ?? '?'}</div>
    `;
    list.appendChild(card);
  }

  renderMapUnits();
}

function selectUnit(unitId) {
  state.selectedUnit = unitId;
  document.getElementById('sel-unit').value = unitId;
  document.querySelectorAll('.unit-card').forEach(c => {
    c.classList.toggle('selected-card', c.getAttribute('data-unit') === unitId);
  });
  loadAvailableOrders(unitId);
}

function onRegionClick(regionId) {
  document.getElementById('inp-target-region').value = regionId;
  addLog(`Selected region: ${regionId}`);
}

// ── Order Form ─────────────────────────────────────────────────────────────
function populateUnitSelector() {
  const sel = document.getElementById('sel-unit');
  const current = sel.value;
  sel.innerHTML = '<option value="">— select unit —</option>';
  for (const u of state.units) {
    if (u.status !== 'ACTIVE') continue;
    const opt = document.createElement('option');
    opt.value = u.id;
    opt.textContent = u.id;
    sel.appendChild(opt);
  }
  if (current) sel.value = current;
}

async function loadAvailableOrders(unitId) {
  if (!unitId) return;
  try {
    const res = await fetch(`${API}/orders/available?unitId=${unitId}&playerId=${state.playerId}`);
    const data = await res.json();
    const sel = document.getElementById('sel-order-type');
    sel.innerHTML = '';
    for (const o of (data.orders || [])) {
      const opt = document.createElement('option');
      opt.value = o;
      opt.textContent = o;
      sel.appendChild(opt);
    }
    updateOrderFormFields();
  } catch (err) {
    console.error('loadAvailableOrders:', err);
  }
}

document.addEventListener('DOMContentLoaded', () => {
  document.getElementById('sel-order-type')?.addEventListener('change', updateOrderFormFields);
  document.getElementById('sel-unit')?.addEventListener('change', e => loadAvailableOrders(e.target.value));
});

function updateOrderFormFields() {
  const orderType = document.getElementById('sel-order-type')?.value || '';
  const showPathIds    = ['ASSIGN_ROUTE','REDIRECT_UNIT'].includes(orderType);
  const showTargetReg  = ['ATTACK_REGION','REINFORCE_REGION','DEPLOY_NAZGUL'].includes(orderType);
  const showTargetPath = ['BLOCK_PATH','SEARCH_PATH','MAIA_ABILITY'].includes(orderType);

  document.getElementById('row-path-ids').style.display    = showPathIds    ? '' : 'none';
  document.getElementById('row-target-region').style.display = showTargetReg ? '' : 'none';
  document.getElementById('row-target-path').style.display  = showTargetPath ? '' : 'none';
}

async function submitOrder() {
  const unitId    = document.getElementById('sel-unit').value;
  const orderType = document.getElementById('sel-order-type').value;
  const pathIdsRaw = document.getElementById('inp-path-ids').value.trim();
  const targetRegion = document.getElementById('inp-target-region').value.trim();
  const targetPath   = document.getElementById('inp-target-path').value.trim();
  const resultEl = document.getElementById('order-result');

  if (!unitId || !orderType) {
    showOrderResult('Select a unit and order type.', false);
    return;
  }

  const order = {
    orderType,
    playerId: state.playerId,
    unitId,
    turn: state.turn,
  };

  if (pathIdsRaw) order.pathIds = pathIdsRaw.split(',').map(s => s.trim()).filter(Boolean);
  if (orderType === 'REDIRECT_UNIT' && pathIdsRaw)
    order.newPathIds = order.pathIds;
  if (targetRegion) order.targetRegion = targetRegion;
  if (targetPath) {
    order.pathId = targetPath;
    order.targetPathId = targetPath;
  }

  try {
    const res = await fetch(`${API}/order`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(order),
    });
    if (res.status === 202) {
      showOrderResult(`✓ Order accepted: ${orderType} for ${unitId}`, true);
      addLog(`Order submitted: ${orderType} → ${unitId}`, 'highlight');
    } else {
      const err = await res.text();
      showOrderResult(`✗ ${err}`, false);
    }
  } catch (err) {
    showOrderResult(`✗ Network error: ${err.message}`, false);
  }
}

function showOrderResult(msg, success) {
  const el = document.getElementById('order-result');
  el.textContent = msg;
  el.className = success ? 'success' : 'error';
  setTimeout(() => el.textContent = '', 4000);
}

// ── Analysis ───────────────────────────────────────────────────────────────
async function fetchAnalysis() {
  const output = document.getElementById('analysis-output');
  output.textContent = 'Loading…';

  try {
    if (state.playerId === 'light') {
      const res = await fetch(`${API}/analysis/routes?playerId=light`);
      const data = await res.json();
      renderRouteAnalysis(data);
    } else {
      const res = await fetch(`${API}/analysis/intercept?playerId=dark`);
      const data = await res.json();
      renderInterceptAnalysis(data);
    }
  } catch (err) {
    output.textContent = 'Analysis unavailable.';
  }
}

function renderRouteAnalysis(data) {
  const output = document.getElementById('analysis-output');
  output.innerHTML = '';

  if (!data.routes?.length) {
    output.textContent = 'No routes available.';
    return;
  }

  for (const route of data.routes) {
    const div = document.createElement('div');
    div.className = `route-item${route.name === data.recommended ? ' recommended' : ''}`;
    div.innerHTML = `
      <div>
        <strong>${route.name}</strong>
        ${route.name === data.recommended ? ' ⭐' : ''}
        <span class="route-score">Risk: ${route.riskScore}</span>
      </div>
      ${route.blockedPaths?.length ? `<div class="route-warnings">Blocked: ${route.blockedPaths.join(', ')}</div>` : ''}
      ${route.threatenedPaths?.length ? `<div class="route-warnings" style="color:orange">Threatened: ${route.threatenedPaths.join(', ')}</div>` : ''}
    `;
    output.appendChild(div);
  }
  if (data.warnings?.length) {
    const warn = document.createElement('div');
    warn.style.color = '#ff9800';
    warn.style.fontSize = '0.75rem';
    warn.style.marginTop = '4px';
    warn.textContent = data.warnings.join(' • ');
    output.appendChild(warn);
  }
}

function renderInterceptAnalysis(data) {
  const output = document.getElementById('analysis-output');
  output.innerHTML = '';

  if (!data.byUnit?.length) {
    output.textContent = 'No interception opportunities.';
    return;
  }

  for (const entry of data.byUnit) {
    const div = document.createElement('div');
    div.className = 'intercept-item';
    div.innerHTML = `
      <div>
        <strong>${entry.unitId}</strong>
        <span class="intercept-score">Score: ${entry.score.toFixed(2)}</span>
      </div>
      <div style="color:#888;font-size:0.75rem">→ ${entry.targetRegion || '—'}</div>
    `;
    output.appendChild(div);
  }
}

// ── Event Log ──────────────────────────────────────────────────────────────
function addLog(msg, type = '') {
  const log = document.getElementById('event-log');
  const entry = document.createElement('div');
  entry.className = `log-entry${type ? ' ' + type : ''}`;
  const time = new Date().toLocaleTimeString('en-US', { hour12: false });
  entry.textContent = `[${time}] ${msg}`;
  log.insertBefore(entry, log.firstChild);
  while (log.children.length > 80) log.removeChild(log.lastChild);
}

// ── Game Over ──────────────────────────────────────────────────────────────
function showGameOver(winner, cause) {
  const overlay = document.getElementById('game-over-overlay');
  const title   = document.getElementById('game-over-title');
  const msg     = document.getElementById('game-over-msg');

  overlay.classList.remove('hidden');
  if (winner === 'FREE_PEOPLES') {
    title.textContent = '☀️ Light Side Wins!';
    title.style.color = '#4a90d9';
    msg.textContent = 'The Ring has been destroyed at Mount Doom. The Shadow is defeated.';
  } else if (winner === 'SHADOW') {
    title.textContent = '🌑 Dark Side Wins!';
    title.style.color = '#cc3333';
    msg.textContent = 'The Ring Bearer was caught. The Age of Shadow begins.';
  } else {
    title.textContent = '⚖ Draw';
    msg.textContent = `After ${state.turn} turns, neither side prevailed. (${cause})`;
  }
  addLog(`GAME OVER — ${winner}: ${cause}`, 'danger');
}

// ── Init ───────────────────────────────────────────────────────────────────
window.addEventListener('load', () => {
  updateOrderFormFields();
});
