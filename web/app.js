// 大老二前端。狀態全部由伺服器推播決定，這裡只負責顯示與送出動作。
'use strict';

// ---------- 常數 ----------

// 與後端 game.Rank / game.Suit 的數值順序一致。
const RANK_TEXT = ['3', '4', '5', '6', '7', '8', '9', '10', 'J', 'Q', 'K', 'A', '2'];
const SUIT_TEXT = ['♣', '♦', '♥', '♠'];
const RED_SUITS = new Set([1, 2]); // 方塊與紅心印紅色

// ---------- 取得元素 ----------

const $ = (id) => document.getElementById(id);

const el = {
  connection: $('connection'),
  screens: {
    name: $('screen-name'),
    lobby: $('screen-lobby'),
    room: $('screen-room'),
    table: $('screen-table'),
  },
  nameForm: $('name-form'),
  nameInput: $('name-input'),
  lobbyMe: $('lobby-me'),
  createForm: $('create-form'),
  roomNameInput: $('room-name-input'),
  roomList: $('room-list'),
  lobbyEmpty: $('lobby-empty'),
  roomTitle: $('room-title'),
  seatList: $('seat-list'),
  leaveBtn: $('leave-btn'),
  startBtn: $('start-btn'),
  startHint: $('start-hint'),
  opponents: $('opponents'),
  tableLabel: $('table-label'),
  tableCards: $('table-cards'),
  myStatus: $('my-status'),
  myHand: $('my-hand'),
  playBtn: $('play-btn'),
  passBtn: $('pass-btn'),
  clearBtn: $('clear-btn'),
  result: $('result'),
  resultList: $('result-list'),
  resultClose: $('result-close'),
  toast: $('toast'),
};

// ---------- 狀態 ----------

const state = {
  ws: null,
  playerID: '',
  name: '',
  room: null,     // 目前所在房間的 roomView
  match: null,    // 目前牌局的 matchView
  selected: [],   // 選取的牌，元素為 {rank, suit}
  resultShown: false,
};

// ---------- 連線 ----------

// 重連的等待時間，連續失敗時逐步拉長，避免狂連伺服器。
let reconnectDelay = 500;

function connect() {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  const ws = new WebSocket(`${proto}//${location.host}/ws`);
  state.ws = ws;

  ws.onopen = () => {
    reconnectDelay = 500;
    el.connection.hidden = true;
    // 重新連上時，如果先前已經取過名字就自動補送，省得再輸入一次。
    if (state.name) send({ action: 'setName', name: state.name });
  };

  ws.onmessage = (ev) => {
    let msg;
    try {
      msg = JSON.parse(ev.data);
    } catch {
      return; // 壞掉的訊息直接忽略
    }
    handle(msg);
  };

  ws.onclose = () => {
    el.connection.hidden = false;
    setTimeout(connect, reconnectDelay);
    reconnectDelay = Math.min(reconnectDelay * 2, 8000);
  };

  // onerror 之後一定會觸發 onclose，重連交給 onclose 處理即可。
  ws.onerror = () => ws.close();
}

function send(msg) {
  if (state.ws && state.ws.readyState === WebSocket.OPEN) {
    state.ws.send(JSON.stringify(msg));
  }
}

// ---------- 處理伺服器訊息 ----------

function handle(msg) {
  switch (msg.type) {
    case 'welcome':
      state.playerID = msg.playerId;
      break;

    case 'lobby':
      state.room = null;
      state.match = null;
      state.selected = [];
      renderLobby(msg.rooms || []);
      showScreen('lobby');
      break;

    case 'room':
      state.room = msg.room;
      state.match = msg.match || null;
      if (state.match) {
        renderTable();
        showScreen('table');
      } else {
        state.selected = [];
        state.resultShown = false;
        renderRoom();
        showScreen('room');
      }
      break;

    case 'kicked':
      toast(msg.message || '你被請出房間了');
      break;

    case 'error':
      toast(msg.message || '動作失敗');
      break;
  }
}

// ---------- 畫面切換 ----------

function showScreen(name) {
  for (const [key, node] of Object.entries(el.screens)) {
    node.hidden = key !== name;
  }
}

// ---------- 大廳 ----------

function renderLobby(rooms) {
  el.lobbyMe.textContent = state.name ? `你好，${state.name}` : '';
  el.roomList.replaceChildren();
  el.lobbyEmpty.hidden = rooms.length > 0;

  for (const room of rooms) {
    const li = document.createElement('li');

    const info = document.createElement('div');
    const name = document.createElement('div');
    name.className = 'room-name';
    name.textContent = room.name;
    const meta = document.createElement('div');
    meta.className = 'room-meta';
    meta.textContent = `${room.seats.length} / 4 人`;
    info.append(name, meta);

    const action = document.createElement('div');
    if (room.started) {
      const badge = document.createElement('span');
      badge.className = 'badge';
      badge.textContent = '遊戲中';
      action.append(badge);
    } else if (room.full) {
      const badge = document.createElement('span');
      badge.className = 'badge';
      badge.textContent = '已滿';
      action.append(badge);
    } else {
      const join = document.createElement('button');
      join.className = 'btn btn-primary btn-sm';
      join.textContent = '加入';
      join.onclick = () => send({ action: 'joinRoom', roomId: room.id });
      action.append(join);
    }

    li.append(info, action);
    el.roomList.append(li);
  }
}

// ---------- 房間 ----------

function renderRoom() {
  const room = state.room;
  if (!room) return;

  el.roomTitle.textContent = `${room.name}（房號 ${room.id}）`;
  el.seatList.replaceChildren();

  // 四個位子都畫出來，空位也要顯示，才看得出還缺幾人。
  for (let i = 0; i < 4; i++) {
    const seat = room.seats[i];
    const li = document.createElement('li');

    const no = document.createElement('span');
    no.className = 'seat-no';
    no.textContent = `${i + 1}.`;

    const name = document.createElement('span');
    name.className = 'seat-name';
    if (seat) {
      name.textContent = seat.name + (seat.isHost ? '（房長）' : '') + (seat.isYou ? '　← 你' : '');
    } else {
      name.classList.add('empty-seat');
      name.textContent = '等待玩家加入…';
    }

    li.append(no, name);

    // 房長可以踢掉除自己以外的人。
    if (seat && room.youAreHost && !seat.isYou) {
      const kick = document.createElement('button');
      kick.className = 'btn btn-danger btn-sm';
      kick.textContent = '踢出';
      kick.onclick = () => send({ action: 'kick', targetId: seat.playerId });
      li.append(kick);
    }

    el.seatList.append(li);
  }

  // 只有房長能開始，而且必須滿四人。
  el.startBtn.hidden = !room.youAreHost;
  el.startBtn.disabled = !room.full;
  if (!room.youAreHost) {
    el.startHint.textContent = '等待房長開始遊戲…';
  } else if (!room.full) {
    el.startHint.textContent = `還需要 ${4 - room.seats.length} 位玩家才能開始`;
  } else {
    el.startHint.textContent = '人數已滿，可以開始了';
  }
}

// ---------- 牌桌 ----------

function renderTable() {
  const m = state.match;
  if (!m) return;

  renderOpponents(m);
  renderTableCards(m);
  renderMyHand(m);
  renderControls(m);

  if (m.over && !state.resultShown) {
    showResult(m);
  }
}

function renderOpponents(m) {
  el.opponents.replaceChildren();

  // 從自己的下一家開始排，讓出牌順序看起來符合直覺。
  const order = [];
  for (let i = 1; i < m.seats.length; i++) {
    order.push(m.seats[(m.yourSeat + i) % m.seats.length]);
  }

  for (const seat of order) {
    const div = document.createElement('div');
    div.className = 'opponent';
    if (seat.isTurn) div.classList.add('is-turn');
    if (seat.rank > 0) div.classList.add('is-out');

    const name = document.createElement('div');
    name.className = 'opponent-name';
    name.textContent = seat.name;

    const meta = document.createElement('div');
    meta.className = 'opponent-meta';
    if (seat.rank > 0) {
      meta.innerHTML = `<span class="opponent-rank">第 ${seat.rank} 名</span>`;
    } else {
      meta.textContent = `${seat.cardCount} 張` + (seat.passed ? '．已 PASS' : '');
    }

    div.append(name, meta);
    el.opponents.append(div);
  }
}

function renderTableCards(m) {
  el.tableCards.replaceChildren();

  if (!m.table) {
    el.tableLabel.textContent = m.over ? '本局結束' : '自由出牌';
    return;
  }
  el.tableLabel.textContent = `檯面：${m.table.type}`;
  for (const c of m.table.cards) {
    el.tableCards.append(cardNode(c));
  }
}

function renderMyHand(m) {
  el.myHand.replaceChildren();

  for (const c of m.yourHand) {
    const node = cardNode(c);
    node.onclick = () => toggleCard(c);
    if (isSelected(c)) node.classList.add('selected');
    el.myHand.append(node);
  }

  const me = m.seats[m.yourSeat];
  if (me && me.rank > 0) {
    el.myStatus.textContent = `你是第 ${me.rank} 名`;
    el.myStatus.className = 'my-status active';
  } else if (m.canPlay) {
    el.myStatus.textContent = m.table ? '輪到你出牌' : '輪到你，可自由出牌';
    el.myStatus.className = 'my-status active';
  } else {
    el.myStatus.textContent = m.over ? '' : '等待其他玩家…';
    el.myStatus.className = 'my-status';
  }
}

function renderControls(m) {
  el.playBtn.disabled = !m.canPlay || state.selected.length === 0;
  el.passBtn.disabled = !m.canPass;
  el.clearBtn.disabled = state.selected.length === 0;
}

// ---------- 選牌 ----------

const sameCard = (a, b) => a.rank === b.rank && a.suit === b.suit;
const isSelected = (c) => state.selected.some((s) => sameCard(s, c));

function toggleCard(c) {
  if (!state.match || !state.match.canPlay) return;

  if (isSelected(c)) {
    state.selected = state.selected.filter((s) => !sameCard(s, c));
  } else {
    state.selected.push({ rank: c.rank, suit: c.suit });
  }
  renderMyHand(state.match);
  renderControls(state.match);
}

function clearSelection() {
  state.selected = [];
  if (state.match) {
    renderMyHand(state.match);
    renderControls(state.match);
  }
}

// ---------- 牌面 ----------

// cardNode 畫出一張牌。左上與右下各印一組點數花色，右下那組轉 180 度，
// 和真實撲克牌一樣。
function cardNode(c) {
  const div = document.createElement('div');
  div.className = 'card';
  if (RED_SUITS.has(c.suit)) div.classList.add('red');

  div.append(corner(c));
  const br = corner(c);
  br.className = 'corner-br';
  div.append(br);

  div.setAttribute('aria-label', `${RANK_TEXT[c.rank]} ${SUIT_TEXT[c.suit]}`);
  return div;
}

function corner(c) {
  const wrap = document.createElement('div');
  const rank = document.createElement('div');
  rank.className = 'rank';
  rank.textContent = RANK_TEXT[c.rank];
  const suit = document.createElement('div');
  suit.className = 'suit';
  suit.textContent = SUIT_TEXT[c.suit];
  wrap.append(rank, suit);
  return wrap;
}

// ---------- 結算 ----------

function showResult(m) {
  state.resultShown = true;
  el.resultList.replaceChildren();

  for (const r of m.rankings) {
    const li = document.createElement('li');
    li.textContent = `${r.name}`;
    if (r.seat === m.yourSeat) li.className = 'me';
    el.resultList.append(li);
  }
  // 沒排進名次的那位就是第四名。
  const last = m.seats.find((s) => s.rank === 0);
  if (last) {
    const li = document.createElement('li');
    li.textContent = `${last.name}（第 4 名）`;
    if (last.seat === m.yourSeat) li.className = 'me';
    el.resultList.append(li);
  }

  el.result.hidden = false;
}

// ---------- 提示訊息 ----------

let toastTimer = 0;

function toast(text) {
  el.toast.textContent = text;
  el.toast.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { el.toast.hidden = true; }, 2600);
}

// ---------- 事件綁定 ----------

el.nameForm.onsubmit = (e) => {
  e.preventDefault();
  const name = el.nameInput.value.trim();
  if (!name) return;
  state.name = name;
  send({ action: 'setName', name });
};

el.createForm.onsubmit = (e) => {
  e.preventDefault();
  send({ action: 'createRoom', name: el.roomNameInput.value.trim() });
  el.roomNameInput.value = '';
};

el.leaveBtn.onclick = () => send({ action: 'leaveRoom' });
el.startBtn.onclick = () => send({ action: 'start' });

el.playBtn.onclick = () => {
  if (state.selected.length === 0) return;
  send({ action: 'play', cards: state.selected });
  state.selected = [];
};

el.passBtn.onclick = () => {
  send({ action: 'pass' });
  state.selected = [];
};

el.clearBtn.onclick = clearSelection;

el.resultClose.onclick = () => {
  el.result.hidden = true;
  send({ action: 'leaveRoom' });
};

// ---------- 啟動 ----------

connect();
