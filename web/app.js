// Playground 平台前端。
//
// 這裡只管所有遊戲共通的事：連線、身分、登入、大廳、房間。
// 一局遊戲長什麼樣、怎麼操作，是各遊戲自己的模組負責（web/games/<id>/），
// 它們透過 Playground.registerGame 掛進來。
'use strict';

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
  gameSelect: $('game-select'),
  roomList: $('room-list'),
  lobbyEmpty: $('lobby-empty'),
  roomTitle: $('room-title'),
  seatList: $('seat-list'),
  leaveBtn: $('leave-btn'),
  startBtn: $('start-btn'),
  startHint: $('start-hint'),
  toast: $('toast'),
};

// ---------- 狀態 ----------

const state = {
  ws: null,
  playerID: '',
  name: '',
  room: null,      // 目前所在房間的 roomView
  games: [],       // 伺服器說有哪些遊戲可以開
  activeGame: '',  // 目前掛載中的遊戲模組 ID

  // resuming 表示這次連上之後要先試著接回先前的身分，
  // 在拿到結果之前不要把畫面切到首頁。
  resuming: false,
};

// ---------- 身分保存 ----------

// token 存在 localStorage，讓重新整理或短暫斷線後還能接回原本的身分
// （包含座位與手牌）。存取包在 try 裡：無痕模式或封鎖網站資料時會丟例外，
// 那種情況就退回「每次都是新玩家」，功能照常只是少了重連。
const TOKEN_KEY = 'bigtwo.token';

function savedToken() {
  try {
    return localStorage.getItem(TOKEN_KEY) || '';
  } catch {
    return '';
  }
}

function saveToken(token) {
  try {
    localStorage.setItem(TOKEN_KEY, token);
  } catch {
    // 存不起來就算了，只是下次無法接回身分。
  }
}

function clearToken() {
  try {
    localStorage.removeItem(TOKEN_KEY);
  } catch {
    // 同上，忽略即可。
  }
}

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
    case 'welcome': {
      state.playerID = msg.playerId;
      state.games = msg.games || [];
      renderGameChoices();

      // 先把舊 token 留著再存新的：伺服器每次連線都發一組新的，
      // 但要接回身分得用舊的那組。
      const previous = savedToken();
      if (msg.token) saveToken(msg.token);

      if (previous && previous !== msg.token) {
        state.resuming = true;
        send({ action: 'resume', token: previous });
      } else {
        showScreen('name');
      }
      break;
    }

    case 'lobby':
      state.resuming = false;
      state.room = null;
      state.match = null;
      state.selected = [];
      renderLobby(msg.rooms || []);
      showScreen('lobby');
      break;

    case 'room':
      state.resuming = false;
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
      if (state.resuming) {
        // 接不回先前的身分（過期、房間沒了、或被別的視窗佔用），
        // 就當作全新的玩家從輸入暱稱開始。
        state.resuming = false;
        state.name = '';
        showScreen('name');
        toast(msg.message || '請重新輸入暱稱');
        break;
      }
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
    meta.textContent = `${room.gameName} ・ ${room.seats.length} / ${room.maxSeats} 人`;
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

  el.roomTitle.textContent = `${room.name}・${room.gameName}（房號 ${room.id}）`;
  el.seatList.replaceChildren();

  // 連空位一起畫出來，才看得出還缺幾人。
  const maxSeats = room.maxSeats || room.seats.length;
  for (let i = 0; i < maxSeats; i++) {
    const seat = room.seats[i];
    const li = document.createElement('li');

    const no = document.createElement('span');
    no.className = 'seat-no';
    no.textContent = `${i + 1}.`;

    const name = document.createElement('span');
    name.className = 'seat-name';
    if (seat) {
      name.textContent = seat.name
        + (seat.isHost ? '（房長）' : '')
        + (seat.isYou ? '　← 你' : '')
        + (seat.offline ? '　（斷線中）' : '');
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
  el.startBtn.disabled = !room.canStart;
  if (!room.youAreHost) {
    el.startHint.textContent = '等待房長開始遊戲…';
  } else if (!room.canStart) {
    el.startHint.textContent = `還需要 ${maxSeats - room.seats.length} 位玩家才能開始`;
  } else {
    el.startHint.textContent = '人數已滿，可以開始了';
  }
}


// ---------- 遊戲模組 ----------

// 各遊戲的畫面模組。web/games/<id>/table.js 會在載入時註冊進來。
const games = {};

// registerGame 讓遊戲模組把自己掛進平台。
//
// mod 需要提供：
//   screen   該遊戲的畫面元素 id
//   mount    進入房間時呼叫一次，用來抓元素、綁事件
//   render   每次收到新狀態時呼叫
//   unmount  離開房間時呼叫，清掉殘留畫面
function registerGame(id, mod) {
  games[id] = mod;
}

// showGame 顯示某個遊戲的畫面並交給它的模組渲染。
function showGame(kindId, view) {
  const mod = games[kindId];
  if (!mod) {
    toast(`還沒有「${kindId}」的畫面`);
    return;
  }

  // 換了遊戲（或第一次進來）才重新掛載，否則每次推播都會清掉選到一半的牌。
  if (state.activeGame !== kindId) {
    unmountGame();
    state.activeGame = kindId;
    mod.mount({
      send: (move) => send({ action: 'move', move }),
      leaveRoom: () => send({ action: 'leaveRoom' }),
    });
  }
  mod.render(view);
  showScreen(mod.screen);
}

// unmountGame 收掉目前掛載的遊戲畫面。
function unmountGame() {
  const mod = games[state.activeGame];
  if (mod && mod.unmount) mod.unmount();
  state.activeGame = '';
}

// renderGameChoices 把可以開的遊戲填進開房的選單。
function renderGameChoices() {
  if (!el.gameSelect) return;
  el.gameSelect.replaceChildren();
  for (const g of state.games) {
    const opt = document.createElement('option');
    opt.value = g.id;
    opt.textContent = g.minSeats === g.maxSeats
      ? `${g.name}（${g.maxSeats} 人）`
      : `${g.name}（${g.minSeats}~${g.maxSeats} 人）`;
    el.gameSelect.append(opt);
  }
}

// 對外只露出註冊用的介面，其餘都是平台內部的事。
window.Playground = { registerGame };

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
  send({
    action: 'createRoom',
    name: el.roomNameInput.value.trim(),
    kindId: el.gameSelect.value,
  });
  el.roomNameInput.value = '';
};

el.leaveBtn.onclick = () => send({ action: 'leaveRoom' });
el.startBtn.onclick = () => send({ action: 'start' });


// ---------- 啟動 ----------

connect();
