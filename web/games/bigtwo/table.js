// 大老二的牌桌畫面。
//
// 平台（app.js）負責登入、大廳、房間與連線；這個檔案只管「一局大老二
// 長什麼樣、怎麼操作」。它透過 Playground.registerGame 掛進平台，
// 平台在進入這種房間時才會載入它。

'use strict';

(function () {
  // 與後端 rules.Rank / rules.Suit 的數值順序一致。
  const RANK_TEXT = ['3', '4', '5', '6', '7', '8', '9', '10', 'J', 'Q', 'K', 'A', '2'];
  const SUIT_TEXT = ['♣', '♦', '♥', '♠'];
  const RED_SUITS = new Set([1, 2]); // 方塊與紅心印紅色

  // 這一局的畫面狀態。平台每次推播都會呼叫 render()，
  // 選到一半的牌要自己記著，不然每次更新都會被清掉。
  let view = null;
  let selected = [];
  let resultShown = false;

  // el 是牌桌用到的元素，在 mount 時抓一次。
  let el = {};

  // send 由平台提供，把動作送給伺服器。
  let send = () => {};

  // ---------- 牌桌 ----------

  function renderTable() {
    const m = view;
    if (!m) return;

    renderOpponents(m);
    renderTableCards(m);
    renderMyHand(m);
    renderControls(m);

    if (m.over && !resultShown) {
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
      if (seat.offline) div.classList.add('is-offline');

      const name = document.createElement('div');
      name.className = 'opponent-name';
      name.textContent = seat.name;

      const meta = document.createElement('div');
      meta.className = 'opponent-meta';
      if (seat.rank > 0) {
        meta.innerHTML = `<span class="opponent-rank">第 ${seat.rank} 名</span>`;
      } else {
        meta.textContent = `${seat.cardCount} 張`
          + (seat.offline ? '．斷線中' : seat.passed ? '．已 PASS' : '');
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

    // 有人斷線時牌局暫停，優先顯示在等誰。
    if (m.waitingFor && m.waitingFor.length > 0) {
      el.myStatus.textContent = `等待 ${m.waitingFor.join('、')} 重新連線…`;
      el.myStatus.className = 'my-status waiting';
      return;
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
    el.playBtn.disabled = !m.canPlay || selected.length === 0;
    el.passBtn.disabled = !m.canPass;
    el.clearBtn.disabled = selected.length === 0;
  }

  // ---------- 選牌 ----------

  const sameCard = (a, b) => a.rank === b.rank && a.suit === b.suit;
  const isSelected = (c) => selected.some((s) => sameCard(s, c));

  function toggleCard(c) {
    if (!view || !view.canPlay) return;

    if (isSelected(c)) {
      selected = selected.filter((s) => !sameCard(s, c));
    } else {
      selected.push({ rank: c.rank, suit: c.suit });
    }
    renderMyHand(view);
    renderControls(view);
  }

  function clearSelection() {
    selected = [];
    if (view) {
      renderMyHand(view);
      renderControls(view);
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
    resultShown = true;
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


  // ---------- 掛進平台 ----------

  // mount 在進入這種房間時被呼叫一次：抓元素、綁事件。
  function mount(ctx) {
    send = ctx.send;
    el = {
      opponents: document.getElementById('opponents'),
      tableLabel: document.getElementById('table-label'),
      tableCards: document.getElementById('table-cards'),
      myStatus: document.getElementById('my-status'),
      myHand: document.getElementById('my-hand'),
      playBtn: document.getElementById('play-btn'),
      passBtn: document.getElementById('pass-btn'),
      clearBtn: document.getElementById('clear-btn'),
      result: document.getElementById('result'),
      resultList: document.getElementById('result-list'),
      resultClose: document.getElementById('result-close'),
    };

    el.playBtn.onclick = () => {
      if (selected.length === 0) return;
      send({ cards: selected });
      selected = [];
    };
    el.passBtn.onclick = () => {
      send({ cards: [] });
      selected = [];
    };
    el.clearBtn.onclick = clearSelection;
    el.resultClose.onclick = () => {
      el.result.hidden = true;
      ctx.leaveRoom();
    };

    selected = [];
    resultShown = false;
  }

  // render 在每次收到新的牌局狀態時被呼叫。
  function render(next) {
    view = next;
    renderTable();
  }

  // unmount 在離開房間時被呼叫，清掉殘留的畫面。
  function unmount() {
    view = null;
    selected = [];
    resultShown = false;
    if (el.result) el.result.hidden = true;
  }

  Playground.registerGame('bigtwo', {
    screen: 'screen-table',
    mount,
    render,
    unmount,
  });
})();
