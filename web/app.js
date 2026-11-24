"use strict";
/* KentGram web client. Plain JS, no dependencies. All user text goes through
   textContent/createTextNode (never innerHTML), and every action calls a real
   backend route: nothing is simulated client-side. Same-origin deployment
   (see deploy/Caddyfile.web.example) so no CORS is required. */
(() => {
  const API = "/api/v1";
  const REACTIONS = ["👍", "❤️", "😂", "😮", "😢", "🔥", "🎉", "👎"];
  const PAGE = 50;

  const ERR = {
    network: "Нет соединения с сервером",
    unauthorized: "Сессия завершена, войдите снова",
    invalid_credentials: "Неверная почта или пароль",
    too_many_login_attempts: "Слишком много попыток входа. Подождите и повторите",
    username_or_email_taken: "Имя пользователя или почта уже заняты",
    invalid_registration_data: "Имя: 3–32 символа (латиница, цифры, _). Пароль: от 10 до 72 символов",
    chat_not_found: "Чат не найден",
    chat_or_reply_not_found: "Чат или сообщение, на которое вы отвечаете, не найдены",
    message_not_found: "Сообщение не найдено или недоступно",
    message_or_chat_not_found: "Сообщение или чат не найдены",
    reaction_limit_reached: "На сообщении уже слишком много разных реакций",
    invalid_emoji: "Недопустимая реакция",
    message_must_be_1_to_4000_characters: "Сообщение должно содержать от 1 до 4000 символов",
    user_not_found: "Пользователь не найден",
    cannot_follow_self: "Нельзя подписаться на себя",
    avatar_url_must_be_https: "Ссылка на аватар должна начинаться с https://",
    profile_field_too_long: "Слишком длинное значение (имя до 64, о себе до 160 символов)",
    query_must_be_2_to_64_characters: "Введите от 2 до 64 символов",
    client_id_conflict: "Повторная отправка отклонена",
    rate: "Слишком много запросов, подождите немного",
  };
  const errText = (e) => (e && (e.status === 429 ? ERR.rate : ERR[e.code])) || `Ошибка: ${e && e.code ? e.code : "неизвестно"}`;

  class ApiError extends Error {
    constructor(status, code, retryAfter) { super(code); this.status = status; this.code = code; this.retryAfter = retryAfter; }
  }

  /* ---------------- tiny DOM helpers ---------------- */
  const $ = (s, r = document) => r.querySelector(s);
  function h(tag, props, ...kids) {
    const el = document.createElement(tag);
    if (props) for (const [k, v] of Object.entries(props)) {
      if (v == null || v === false) continue;
      if (k === "class") el.className = v;
      else if (k === "text") el.textContent = v;
      else if (k === "dataset") Object.assign(el.dataset, v);
      else if (k.startsWith("on")) el.addEventListener(k.slice(2), v);
      else el.setAttribute(k, v === true ? "" : v);
    }
    for (const kid of kids.flat(Infinity)) {
      if (kid == null || kid === false) continue;
      el.append(kid.nodeType ? kid : String(kid));
    }
    return el;
  }
  const svg = (path) => {
    const s = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    s.setAttribute("viewBox", "0 0 24 24"); s.setAttribute("aria-hidden", "true");
    const p = document.createElementNS("http://www.w3.org/2000/svg", "path");
    p.setAttribute("d", path); s.append(p); return s;
  };
  const ICON = {
    plus: "M12 5v14M5 12h14", gear: "M12 15a3 3 0 100-6 3 3 0 000 6zM19.4 15a1.7 1.7 0 00.3 1.8l.1.1a2 2 0 11-2.8 2.8l-.1-.1a1.7 1.7 0 00-1.8-.3 1.7 1.7 0 00-1 1.5V21a2 2 0 11-4 0v-.1a1.7 1.7 0 00-1.1-1.5 1.7 1.7 0 00-1.8.3l-.1.1a2 2 0 11-2.8-2.8l.1-.1a1.7 1.7 0 00.3-1.8 1.7 1.7 0 00-1.5-1H3a2 2 0 110-4h.1a1.7 1.7 0 001.5-1.1 1.7 1.7 0 00-.3-1.8l-.1-.1a2 2 0 112.8-2.8l.1.1a1.7 1.7 0 001.8.3H9a1.7 1.7 0 001-1.5V3a2 2 0 114 0v.1a1.7 1.7 0 001 1.5 1.7 1.7 0 001.8-.3l.1-.1a2 2 0 112.8 2.8l-.1.1a1.7 1.7 0 00-.3 1.8V9a1.7 1.7 0 001.5 1H21a2 2 0 110 4h-.1a1.7 1.7 0 00-1.5 1z",
    back: "M15 18l-6-6 6-6", send: "M22 2L11 13M22 2l-7 20-4-9-9-4 20-7z", x: "M18 6L6 18M6 6l12 12",
  };
  const iconBtn = (name, label, onclick, extra = "") => h("button", { class: "icon-btn " + extra, "aria-label": label, title: label, type: "button", onclick }, svg(ICON[name]));

  const uuid = () => (crypto.randomUUID ? crypto.randomUUID() : "c" + Date.now().toString(36) + Math.random().toString(36).slice(2, 12));
  const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
  const isCoarse = () => matchMedia("(pointer: coarse)").matches;

  function toast(text, opts = {}) {
    const el = h("div", { class: "toast" + (opts.error ? " err" : "") }, text);
    if (opts.action) {
      el.style.pointerEvents = "auto";
      el.append(" ", h("button", { class: "btn ghost", type: "button", onclick: opts.action.run }, opts.action.label));
    }
    $("#toasts").append(el);
    if (!opts.sticky) setTimeout(() => el.remove(), opts.ms || 3500);
    return el;
  }
  const fail = (e) => toast(errText(e), { error: true });

  const fmtTime = (d) => new Date(d).toLocaleTimeString("ru", { hour: "2-digit", minute: "2-digit" });
  const sameDay = (a, b) => a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
  function dayLabel(d) {
    const dt = new Date(d), now = new Date(), y = new Date(now); y.setDate(now.getDate() - 1);
    if (sameDay(dt, now)) return "Сегодня";
    if (sameDay(dt, y)) return "Вчера";
    return dt.toLocaleDateString("ru", { day: "numeric", month: "long", year: dt.getFullYear() === now.getFullYear() ? undefined : "numeric" });
  }
  function listTime(d) {
    if (!d) return "";
    const dt = new Date(d);
    return sameDay(dt, new Date()) ? fmtTime(dt) : dt.toLocaleDateString("ru", { day: "2-digit", month: "2-digit" });
  }

  const URL_RE = /\bhttps?:\/\/[^\s<>"']+/gi;
  function linkify(text) {
    const out = []; let last = 0;
    for (const m of text.matchAll(URL_RE)) {
      let url = m[0]; const trail = url.match(/[.,;:!?)\]]+$/);
      if (trail) url = url.slice(0, -trail[0].length);
      if (m.index > last) out.push(text.slice(last, m.index));
      out.push(h("a", { href: url, target: "_blank", rel: "noopener noreferrer nofollow", text: url }));
      last = m.index + url.length;
    }
    if (last < text.length) out.push(text.slice(last));
    return out;
  }

  function avatar(u, size) {
    const name = u.display_name || u.username || "?";
    let hash = 0; for (const c of u.username || name) hash = (hash * 31 + c.charCodeAt(0)) >>> 0;
    const el = h("span", { class: "avatar s" + size, dataset: { uid: u.id || "" } });
    el.style.background = `hsl(${hash % 360} 32% 30%)`;
    el.append(h("span", { text: [...name.trim()][0]?.toUpperCase() || "?" }));
    if (u.avatar_url && u.avatar_url.startsWith("https://")) {
      const img = h("img", { src: u.avatar_url, alt: "", loading: "lazy", decoding: "async", referrerpolicy: "no-referrer" });
      img.addEventListener("error", () => img.remove());
      el.append(img);
    }
    if (u.id && S.online.has(u.id)) el.classList.add("online");
    return el;
  }

  /* ---------------- tokens + api ---------------- */
  const TK = "kg.tokens";
  let memTokens = null;
  function loadTokens() {
    try { const v = localStorage.getItem(TK); return v ? JSON.parse(v) : memTokens; } catch { return memTokens; }
  }
  function saveTokens(j) {
    memTokens = { access: j.access_token, refresh: j.refresh_token, exp: Date.now() + (j.expires_in || 900) * 1000 };
    try { localStorage.setItem(TK, JSON.stringify(memTokens)); } catch { /* private mode: memory only */ }
  }
  function clearTokens() { memTokens = null; try { localStorage.removeItem(TK); } catch { /* ignore */ } }

  let refreshing = null;
  function refreshTokens(staleAccess) {
    // Single flight per tab, and a Web Lock across tabs: refresh tokens rotate,
    // so two tabs refreshing with the same token would look like token theft.
    if (!refreshing) refreshing = doRefresh(staleAccess).finally(() => { refreshing = null; });
    return refreshing;
  }
  async function doRefresh(staleAccess) {
    const run = async () => {
      const t = loadTokens();
      if (!t) return false;
      if (t.access !== staleAccess) return true; // another tab already rotated it
      let res;
      try {
        res = await fetch(API + "/auth/refresh", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ refresh_token: t.refresh }) });
      } catch { throw new ApiError(0, "network"); }
      if (res.status === 429 || res.status >= 500) throw new ApiError(res.status, "refresh_unavailable");
      if (!res.ok) return false;
      saveTokens(await res.json());
      return true;
    };
    return navigator.locks ? navigator.locks.request("kg-refresh", run) : run();
  }

  async function api(path, opt = {}) {
    const { method = "GET", body, auth = true } = opt;
    for (let attempt = 0; attempt < 2; attempt++) {
      const t = auth ? loadTokens() : null;
      if (auth && !t) throw new ApiError(401, "unauthorized");
      const ctl = new AbortController();
      const timer = setTimeout(() => ctl.abort(), 20000);
      let res;
      try {
        const headers = {};
        if (body !== undefined) headers["Content-Type"] = "application/json";
        if (t) headers.Authorization = "Bearer " + t.access;
        res = await fetch(API + path, { method, headers, signal: ctl.signal, body: body !== undefined ? JSON.stringify(body) : undefined });
      } catch { throw new ApiError(0, "network"); } finally { clearTimeout(timer); }

      if (res.status === 401 && auth) {
        if (attempt === 0 && (await refreshTokens(t.access))) continue;
        signOut();
        throw new ApiError(401, "unauthorized");
      }
      let data = null;
      try { data = await res.json(); } catch { /* empty body */ }
      if (!res.ok) throw new ApiError(res.status, (data && data.error) || "http_" + res.status, res.headers.get("Retry-After"));
      return data;
    }
  }

  /* ---------------- state ---------------- */
  const S = {
    me: null, chats: [], contacts: null, tab: "chats", filter: "",
    online: new Set(), typing: new Map(), active: null, route: { name: "list" },
  };
  let cs = null;        // state of the open chat only: history is re-fetched on open, never stale
  let openSeq = 0;
  const drafts = new Map();

  const chatById = (id) => S.chats.find((c) => c.id === id);
  const chatName = (c) => c.peer_display_name || c.peer_username;
  const peerOf = (c) => ({ id: c.peer_id, username: c.peer_username, display_name: c.peer_display_name, avatar_url: c.peer_avatar_url });

  /* ---------------- router ---------------- */
  function parseRoute() {
    const [name, arg] = location.hash.replace(/^#\/?/, "").split("/");
    return { name: name || "list", arg: arg ? decodeURIComponent(arg) : null };
  }
  function go(name, arg) {
    const hash = "#/" + (name === "list" ? "" : name + (arg != null ? "/" + encodeURIComponent(arg) : ""));
    history.pushState({ kg: 1 }, "", hash);
    route();
  }
  function back() {
    if (history.state && history.state.kg) history.back();
    else { history.replaceState({ kg: 0 }, "", "#/"); route(); }
  }
  function leaveChat() {
    if (cs) { const ta = $("#ta"); if (ta) drafts.set(cs.id, ta.value); }
    openSeq++; cs = null; S.active = null;
  }
  function route() {
    if (!S.me) return;
    const r = (S.route = parseRoute());
    document.body.classList.toggle("show-main", r.name !== "list");
    if (r.name === "chat" && Number.isInteger(+r.arg) && +r.arg > 0) {
      if (!(cs && cs.id === +r.arg)) { leaveChat(); openChat(+r.arg); }
    } else {
      leaveChat();
      if (r.name === "user" && r.arg) showProfile(r.arg);
      else if (r.name === "settings") showSettings();
      else showPlaceholder();
    }
    renderList();
  }

  /* ---------------- auth screen ---------------- */
  function mountAuth(message) {
    document.body.classList.remove("show-main");
    let mode = "login";
    const err = h("div", { class: "form-error", role: "alert" }, message || "");
    const email = h("input", { class: "input", type: "email", autocomplete: "email", required: true, id: "f-email" });
    const user = h("input", { class: "input", autocomplete: "username", minlength: 3, maxlength: 32, pattern: "[A-Za-z0-9_]{3,32}", id: "f-user" });
    const pass = h("input", { class: "input", type: "password", minlength: 10, maxlength: 72, required: true, id: "f-pass" });
    const userField = h("div", { class: "field hidden" }, h("label", { for: "f-user", text: "Имя пользователя" }), user);
    const hint = h("div", { class: "hint hidden", text: "Пароль — от 10 символов." });
    const submit = h("button", { class: "btn", type: "submit", text: "Войти" });
    const loginTab = h("button", { type: "button", "aria-pressed": "true", text: "Вход" });
    const regTab = h("button", { type: "button", "aria-pressed": "false", text: "Регистрация" });
    const setMode = (m) => {
      mode = m; err.textContent = "";
      loginTab.setAttribute("aria-pressed", m === "login"); regTab.setAttribute("aria-pressed", m === "register");
      userField.classList.toggle("hidden", m === "login"); hint.classList.toggle("hidden", m === "login");
      user.required = m === "register"; pass.autocomplete = m === "login" ? "current-password" : "new-password";
      submit.textContent = m === "login" ? "Войти" : "Создать аккаунт";
    };
    loginTab.onclick = () => setMode("login"); regTab.onclick = () => setMode("register");
    setMode("login");
    const form = h("form", { class: "auth-card", novalidate: false, onsubmit: async (e) => {
      e.preventDefault(); err.textContent = ""; submit.disabled = true;
      try {
        const body = mode === "login"
          ? { email: email.value.trim(), password: pass.value, device_name: "Web" }
          : { username: user.value.trim(), email: email.value.trim(), password: pass.value, device_name: "Web" };
        const res = await api(mode === "login" ? "/auth/login" : "/auth/register", { method: "POST", body, auth: false });
        saveTokens(res); S.me = res.user; await startApp();
      } catch (ex) { err.textContent = errText(ex); } finally { submit.disabled = false; }
    } },
      h("div", { class: "brand" }, h("span", { class: "brand-mark", text: "K" }), "KentGram"),
      h("div", { class: "seg" }, loginTab, regTab),
      userField,
      h("div", { class: "field" }, h("label", { for: "f-email", text: "Почта" }), email),
      h("div", { class: "field" }, h("label", { for: "f-pass", text: "Пароль" }), pass),
      hint, err, submit);
    $("#app").replaceChildren(h("div", { class: "auth" }, form));
    email.focus();
  }

  /* ---------------- app shell ---------------- */
  function mountApp() {
    const filter = h("input", { class: "input", type: "search", placeholder: "Фильтр чатов", "aria-label": "Фильтр чатов", id: "filter",
      oninput: (e) => { S.filter = e.target.value.trim().toLowerCase(); renderList(); } });
    const tabs = h("div", { class: "tabs", role: "tablist" },
      h("button", { role: "tab", "aria-selected": "true", id: "tab-chats", onclick: () => setTab("chats"), text: "Чаты" }),
      h("button", { role: "tab", "aria-selected": "false", id: "tab-contacts", onclick: () => setTab("contacts"), text: "Контакты" }));
    const side = h("aside", { class: "sidebar" },
      h("div", { class: "side-head" },
        h("button", { class: "icon-btn", "aria-label": "Профиль и настройки", onclick: () => go("settings") }, avatar(S.me, 36)),
        h("div", { class: "side-title", text: "KentGram" }),
        iconBtn("plus", "Новый чат", openNewChat)),
      h("div", { class: "side-search" }, filter), tabs,
      h("div", { class: "list", id: "list" }));
    const main = h("main", { class: "main", id: "main" });
    $("#app").replaceChildren(h("div", { class: "layout" }, side, main));
  }
  function setTab(t) {
    S.tab = t;
    $("#tab-chats").setAttribute("aria-selected", t === "chats"); $("#tab-contacts").setAttribute("aria-selected", t === "contacts");
    $("#filter").placeholder = t === "chats" ? "Фильтр чатов" : "Фильтр контактов";
    if (t === "contacts" && !S.contacts) loadContacts();
    renderList();
  }
  async function loadContacts() {
    try { S.contacts = (await api("/contacts")).contacts || []; } catch (e) { fail(e); S.contacts = []; }
    renderList();
  }
  async function loadChats() {
    try { S.chats = (await api("/chats")).chats || []; renderList(); updateTitle(); } catch (e) { if (e.status !== 401) fail(e); }
  }
  let loadChatsTimer = 0;
  const loadChatsSoon = () => { clearTimeout(loadChatsTimer); loadChatsTimer = setTimeout(loadChats, 400); };
  function updateTitle() {
    const n = S.chats.reduce((a, c) => a + (c.unread_count || 0), 0);
    document.title = n ? `(${n}) KentGram` : "KentGram";
  }

  let listQueued = false;
  function renderList() { if (listQueued) return; listQueued = true; requestAnimationFrame(() => { listQueued = false; drawList(); }); }
  function drawList() {
    const box = $("#list"); if (!box) return;
    let rows;
    if (S.tab === "chats") {
      const items = S.chats.filter((c) => !S.filter || (chatName(c) + " " + c.peer_username + " " + c.last_message).toLowerCase().includes(S.filter));
      rows = items.map((c) => {
        const typing = S.typing.has(c.id);
        return h("button", { class: "row", type: "button", "aria-current": S.active === c.id ? "true" : null, onclick: () => go("chat", c.id) },
          avatar(peerOf(c), 48),
          h("div", { class: "row-body" },
            h("div", { class: "row-top" }, h("div", { class: "row-name", text: chatName(c) }), h("div", { class: "row-time", text: listTime(c.last_message_at) })),
            h("div", { class: "row-bottom" },
              h("div", { class: "row-sub" + (typing ? " typing" : ""), text: typing ? "печатает…" : (c.last_message || "Нет сообщений") }),
              c.unread_count > 0 && h("span", { class: "badge", text: c.unread_count > 99 ? "99+" : c.unread_count }))));
      });
      if (!rows.length) rows = [h("div", { class: "empty", text: S.filter ? "Ничего не найдено" : "Чатов пока нет. Нажмите «+», чтобы найти человека и написать ему." })];
    } else if (S.contacts == null) {
      rows = [h("div", { class: "empty", text: "Загрузка…" })];
    } else {
      const items = S.contacts.filter((u) => !S.filter || (u.username + " " + u.display_name).toLowerCase().includes(S.filter));
      rows = items.map((u) => userRow(u));
      if (!rows.length) rows = [h("div", { class: "empty", text: S.filter ? "Ничего не найдено" : "Вы пока ни на кого не подписаны." })];
    }
    box.replaceChildren(...rows);
  }
  const userRow = (u, after) => h("button", { class: "row", type: "button", onclick: () => { after && after(); go("user", u.username); } },
    avatar(u, 48), h("div", { class: "row-body" }, h("div", { class: "row-name", text: u.display_name || u.username }), h("div", { class: "row-sub", text: "@" + u.username })));

  const paneHead = (title, extra) => h("div", { class: "pane-head" }, iconBtn("back", "Назад", back, "back"), h("h2", { text: title }), extra);
  function setMain(...nodes) { const m = $("#main"); if (m) m.replaceChildren(...nodes); }
  function showPlaceholder() {
    setMain(h("div", { class: "placeholder" }, h("div", null, h("div", { class: "brand-mark", text: "K" }), h("div", { text: "Выберите чат, чтобы начать переписку" }))));
  }

  /* ---------------- dialogs ---------------- */
  function dialog(title, ...body) {
    const d = h("dialog");
    const close = () => d.close();
    d.append(h("div", { class: "dlg" }, h("div", { class: "dlg-head" }, h("span", { text: title }), iconBtn("x", "Закрыть", close)), h("div", { class: "dlg-body" }, ...body)));
    d.addEventListener("click", (e) => { if (e.target === d) d.close(); });
    d.addEventListener("close", () => d.remove());
    document.body.append(d); d.showModal();
    return d;
  }

  function openNewChat() {
    const results = h("div", { class: "dlg-body-inner" });
    const input = h("input", { class: "input", type: "search", placeholder: "Имя пользователя или имя", "aria-label": "Поиск людей", autocomplete: "off" });
    const d = dialog("Новый чат", input, results);
    let timer = 0, seq = 0;
    input.addEventListener("input", () => {
      clearTimeout(timer);
      const q = input.value.trim();
      if (q.length < 2) { results.replaceChildren(); return; }
      timer = setTimeout(async () => {
        const my = ++seq;
        try {
          const { users } = await api("/users/search?q=" + encodeURIComponent(q));
          if (my !== seq) return;
          results.replaceChildren(...(users.length ? users.map((u) => userRow(u, () => d.close())) : [h("div", { class: "empty", text: "Никого не нашли" })]));
        } catch (e) { if (my === seq) results.replaceChildren(h("div", { class: "empty", text: errText(e) })); }
      }, 300);
    });
    input.focus();
  }

  /* ---------------- profile / settings ---------------- */
  async function showProfile(username) {
    const my = ++openSeq;
    setMain(paneHead("Профиль"), h("div", { class: "pane" }, h("div", { class: "pane-inner" }, h("div", { class: "empty", text: "Загрузка…" }))));
    let p;
    try { p = await api("/users/" + encodeURIComponent(username)); } catch (e) {
      if (my !== openSeq) return;
      setMain(paneHead("Профиль"), h("div", { class: "pane" }, h("div", { class: "pane-inner" }, h("div", { class: "empty", text: errText(e) })))); return;
    }
    if (my !== openSeq) return;
    const isMe = p.id === S.me.id;
    const followBtn = h("button", { class: "btn ghost", type: "button" });
    const paintFollow = () => { followBtn.textContent = p.is_following ? "Отписаться" : "Подписаться"; };
    paintFollow();
    followBtn.onclick = async () => {
      followBtn.disabled = true;
      try {
        await api(`/users/${p.id}/follow`, { method: p.is_following ? "DELETE" : "POST" });
        p.is_following = !p.is_following; p.followers += p.is_following ? 1 : -1; paintFollow(); statFollowers.textContent = p.followers; S.contacts = null;
        if (S.tab === "contacts") loadContacts();
      } catch (e) { fail(e); } finally { followBtn.disabled = false; }
    };
    const statFollowers = h("b", { text: p.followers });
    const msgBtn = h("button", { class: "btn", type: "button", text: "Написать", onclick: async () => {
      msgBtn.disabled = true;
      try { const { chat_id } = await api("/chats/direct", { method: "POST", body: { user_id: p.id } }); await loadChats(); go("chat", chat_id); }
      catch (e) { fail(e); msgBtn.disabled = false; }
    } });
    setMain(paneHead("Профиль"), h("div", { class: "pane" }, h("div", { class: "pane-inner" }, h("div", { class: "card profile" },
      avatar(p, 96), h("h2", { text: p.display_name || p.username }), h("div", { class: "muted", text: "@" + p.username }),
      p.bio && h("div", { text: p.bio }),
      h("div", { class: "stats" }, h("div", null, statFollowers, h("span", { class: "muted", text: "подписчиков" })), h("div", null, h("b", { text: p.following }), h("span", { class: "muted", text: "подписок" }))),
      h("div", { class: "actions" }, isMe ? h("button", { class: "btn", type: "button", text: "Настройки", onclick: () => go("settings") }) : [msgBtn, followBtn])))));
  }

  async function showSettings() {
    const my = ++openSeq;
    const me = S.me;
    const err = h("div", { class: "form-error", role: "alert" });
    const dn = h("input", { class: "input", maxlength: 64, value: me.display_name || "", id: "s-dn" });
    const bio = h("textarea", { class: "input", maxlength: 160, id: "s-bio" }); bio.value = me.bio || "";
    const av = h("input", { class: "input", type: "url", maxlength: 2048, placeholder: "https://…", value: me.avatar_url || "", id: "s-av" });
    const save = h("button", { class: "btn", type: "submit", text: "Сохранить" });
    const form = h("form", { class: "card", onsubmit: async (e) => {
      e.preventDefault(); err.textContent = ""; save.disabled = true;
      try {
        S.me = Object.assign(S.me, await api("/me/profile", { method: "PATCH", body: { display_name: dn.value.trim(), bio: bio.value.trim(), avatar_url: av.value.trim() } }));
        toast("Профиль сохранён"); mountSideAvatar();
      } catch (ex) { err.textContent = errText(ex); } finally { save.disabled = false; }
    } },
      h("h3", { text: "Профиль" }),
      h("div", { class: "field" }, h("label", { for: "s-dn", text: "Отображаемое имя" }), dn),
      h("div", { class: "field" }, h("label", { for: "s-bio", text: "О себе" }), bio),
      h("div", { class: "field" }, h("label", { for: "s-av", text: "Ссылка на аватар" }), av, h("div", { class: "hint", text: "Только https. Загрузка файлов пока не поддерживается сервером." })),
      err, save);
    const sess = h("div", { class: "card" }, h("h3", { text: "Устройства" }), h("div", { class: "muted", text: "Загрузка…" }));
    const logout = h("button", { class: "btn danger", type: "button", text: "Выйти", onclick: async () => {
      logout.disabled = true;
      try { await api("/auth/logout", { method: "POST" }); } catch { /* token may already be gone */ }
      signOut();
    } });
    setMain(paneHead("Настройки"), h("div", { class: "pane" }, h("div", { class: "pane-inner" },
      h("div", { class: "muted", text: "@" + me.username + " · " + me.email }), form, sess, h("div", { class: "card" }, logout))));
    try {
      const { sessions } = await api("/sessions");
      if (my !== openSeq) return;
      const paint = (list) => sess.replaceChildren(h("h3", { text: "Устройства" }),
        ...list.map((s) => h("div", { class: "sess" },
          h("div", null, h("div", { text: (s.device_name || "Устройство") + (s.current ? " · это устройство" : "") }), h("div", { class: "muted", text: `${s.ip || ""} · ${new Date(s.last_used_at).toLocaleString("ru")}` })),
          !s.current && h("button", { class: "btn ghost", type: "button", text: "Завершить", onclick: async () => {
            try { await api("/sessions/" + s.id, { method: "DELETE" }); paint(list.filter((x) => x.id !== s.id)); } catch (e) { fail(e); }
          } }))),
        list.some((s) => !s.current) && h("button", { class: "btn ghost", type: "button", text: "Завершить все остальные", onclick: async () => {
          try { await api("/sessions/revoke-others", { method: "POST" }); paint(list.filter((x) => x.current)); } catch (e) { fail(e); }
        } }));
      paint(sessions || []);
    } catch (e) { if (my === openSeq) sess.replaceChildren(h("h3", { text: "Устройства" }), h("div", { class: "muted", text: errText(e) })); }
  }
  function mountSideAvatar() {
    const b = $(".side-head .icon-btn"); if (b) b.replaceChildren(avatar(S.me, 36));
  }

  /* ---------------- chat view ---------------- */
  const newCS = (id) => ({ id, msgs: [], els: new Map(), peerRead: 0, myRead: 0, hasMore: true, loading: false, loaded: false, queue: [], pins: [], pinIdx: 0, replyTo: null, editing: null, unseen: 0, lastTyping: 0 });
  const keyOf = (m) => (m.id != null ? m.id : "c:" + m.client_id);
  const isPending = (m) => m.id == null;

  async function openChat(id) {
    const seq = ++openSeq;
    S.active = id;
    cs = newCS(id);
    const chat = chatById(id);
    buildChatPane(chat);
    try {
      const [hist, pins] = await Promise.all([
        api(`/chats/${id}/messages`),
        api(`/chats/${id}/pins`).catch(() => ({ messages: [] })),
      ]);
      if (seq !== openSeq) return;
      cs.msgs = (hist.messages || []).map(normalize);
      cs.peerRead = hist.peer_last_read_message_id || 0;
      cs.hasMore = cs.msgs.length >= PAGE;
      cs.pins = pins.messages || [];
      cs.myRead = lastRealId();
      cs.loaded = true;
      drawMessages();
      scrollBottom();
      drawPin();
      const q = cs.queue; cs.queue = []; q.forEach(handleChatEvent);
      if (chat) { chat.unread_count = 0; renderList(); updateTitle(); }
    } catch (e) {
      if (seq !== openSeq) return;
      $("#wrap").replaceChildren(h("div", { class: "empty", text: errText(e) }));
    }
  }
  const normalize = (m) => { m.reactions = m.reactions || []; m.is_mine = m.sender_id === S.me.id; return m; };
  const lastRealId = () => { for (let i = cs.msgs.length - 1; i >= 0; i--) if (!isPending(cs.msgs[i])) return cs.msgs[i].id; return 0; };

  function buildChatPane(chat) {
    const status = h("div", { class: "who-status", id: "status" });
    const who = chat
      ? h("button", { class: "who", type: "button", onclick: () => go("user", chat.peer_username) }, h("div", { class: "who-name", text: chatName(chat) }), status)
      : h("div", { class: "who" }, h("div", { class: "who-name", text: "Чат" }), status);
    const head = h("div", { class: "chat-head" }, iconBtn("back", "Назад к чатам", back, "back"), chat && avatar(peerOf(chat), 36), who);
    const ta = h("textarea", { id: "ta", rows: 1, maxlength: 4000, placeholder: "Сообщение", "aria-label": "Сообщение", enterkeyhint: "send" });
    const sendBtn = h("button", { class: "send", type: "submit", "aria-label": "Отправить", disabled: true }, svg(ICON.send));
    const form = h("form", { class: "composer", onsubmit: (e) => { e.preventDefault(); submitComposer(); } }, ta, sendBtn);
    const grow = () => { ta.style.height = "auto"; ta.style.height = Math.min(ta.scrollHeight + 2, 168) + "px"; sendBtn.disabled = !ta.value.trim(); };
    ta.addEventListener("input", () => {
      grow();
      if (ta.value.trim() && cs && Date.now() - cs.lastTyping > 3000) { cs.lastTyping = Date.now(); api(`/chats/${cs.id}/typing`, { method: "POST" }).catch(() => {}); }
    });
    ta.addEventListener("keydown", (e) => {
      if (e.key === "Enter" && !e.shiftKey && !e.isComposing && !isCoarse()) { e.preventDefault(); submitComposer(); }
      if (e.key === "Escape" && cs && (cs.replyTo || cs.editing)) { clearContext(true); }
    });
    ta.value = drafts.get(chat ? chat.id : cs.id) || "";
    const wrap = h("div", { class: "chat-wrap", id: "wrap" }, h("div", { class: "loader", text: "Загрузка…" }));
    const messages = h("div", { class: "messages", id: "messages" }, h("div", { class: "spacer" }), wrap);
    messages.addEventListener("scroll", onScroll, { passive: true });
    const jump = h("button", { class: "jump hidden", id: "jump", type: "button", onclick: () => { scrollBottom(); } }, "Вниз ↓");
    setMain(h("section", { class: "chat" }, head, h("div", { id: "pinhost" }), h("div", { class: "scroll" }, messages, jump), h("div", { class: "composer-zone" }, h("div", { id: "ctxhost" }), form)));
    paintStatus(); grow();
    if (!isCoarse()) ta.focus();
  }
  function paintStatus() {
    const el = $("#status"); if (!el || !cs) return;
    const c = chatById(cs.id);
    el.className = "who-status";
    if (!c) { el.textContent = ""; return; }
    if (S.typing.has(cs.id)) { el.textContent = "печатает…"; el.classList.add("typing"); }
    else if (S.online.has(c.peer_id)) { el.textContent = "в сети"; el.classList.add("on"); }
    else el.textContent = "не в сети";
  }

  const nearBottom = () => { const m = $("#messages"); return !m || m.scrollHeight - m.scrollTop - m.clientHeight < 140; };
  function scrollBottom() {
    const m = $("#messages"); if (!m) return;
    m.scrollTop = m.scrollHeight;
    if (cs) cs.unseen = 0;
    $("#jump")?.classList.add("hidden");
  }
  function onScroll() {
    const m = $("#messages"); if (!m || !cs) return;
    if (nearBottom()) { cs.unseen = 0; $("#jump")?.classList.add("hidden"); scheduleRead(); }
    else $("#jump")?.classList.remove("hidden");
    if (m.scrollTop < 80 && cs.loaded && cs.hasMore && !cs.loading) loadOlder();
  }
  function paintJump() {
    const j = $("#jump"); if (!j || !cs) return;
    j.replaceChildren("Вниз ↓", cs.unseen > 0 ? h("b", { text: cs.unseen }) : null);
    j.classList.toggle("hidden", nearBottom());
  }

  async function loadOlder() {
    const first = cs.msgs.find((m) => !isPending(m));
    if (!first) return;
    const my = cs; cs.loading = true;
    try {
      const { messages } = await api(`/chats/${cs.id}/messages?before_id=${first.id}`);
      if (cs !== my) return;
      const older = (messages || []).map(normalize);
      cs.hasMore = older.length >= PAGE;
      const m = $("#messages"), before = m.scrollHeight;
      const have = new Set(cs.msgs.map((x) => x.id));
      cs.msgs = older.filter((x) => !have.has(x.id)).concat(cs.msgs);
      drawMessages();
      m.scrollTop += m.scrollHeight - before; // keep the viewport anchored
    } catch (e) { if (cs === my) fail(e); } finally { if (cs === my) cs.loading = false; }
  }

  function drawMessages() {
    const wrap = $("#wrap"); if (!wrap || !cs) return;
    cs.els.clear();
    const nodes = []; let day = null;
    if (cs.hasMore) nodes.push(h("div", { class: "loader", text: "Загрузка истории…" }));
    else if (!cs.msgs.length) nodes.push(h("div", { class: "empty", text: "Сообщений пока нет. Напишите первым." }));
    for (const m of cs.msgs) {
      const d = new Date(m.created_at);
      if (!day || !sameDay(day, d)) { nodes.push(h("div", { class: "day", text: dayLabel(d) })); day = d; }
      const el = msgEl(m); cs.els.set(keyOf(m), el); nodes.push(el);
    }
    wrap.replaceChildren(...nodes);
  }

  function msgEl(m) {
    const mine = m.sender_id === S.me.id, pending = isPending(m), failed = m.status === "failed";
    const el = h("div", { class: "msg" + (mine ? " mine" : "") + (pending && !failed ? " pending" : "") + (failed ? " failed" : ""), dataset: { id: m.id ?? "" }, oncontextmenu: (e) => { e.preventDefault(); openMenu(m); } });
    const b = h("div", { class: "bubble" });
    if (m.forwarded_from_user_id) b.append(h("div", { class: "fwd", text: "Переслано от " + (m.forwarded_from_username ? "@" + m.forwarded_from_username : "пользователя") }));
    if (m.reply_to_message_id) {
      b.append(h("button", { class: "quote", type: "button", onclick: () => jumpTo(m.reply_to_message_id) },
        h("b", { text: m.reply_to_sender_username ? "@" + m.reply_to_sender_username : "Ответ" }),
        h("span", { text: m.reply_to_body || "Сообщение удалено" })));
    }
    b.append(h("div", { class: "text" }, linkify(m.body)));
    const ticks = mine ? h("span", { class: "ticks" + (failed ? " bad" : !pending && m.id <= cs.peerRead ? " read" : ""), text: failed ? "!" : pending ? "…" : m.id <= cs.peerRead ? "✓✓" : "✓", title: failed ? "Не отправлено" : pending ? "Отправляется" : m.id <= cs.peerRead ? "Прочитано" : "Отправлено" }) : null;
    b.append(h("div", { class: "meta" },
      h("button", { class: "more", type: "button", "aria-label": "Действия с сообщением", onclick: (e) => { e.stopPropagation(); openMenu(m); }, text: "⋯" }),
      m.pinned && h("span", { title: "Закреплено", text: "📌" }), m.edited_at && h("span", { text: "изм." }), h("span", { text: fmtTime(m.created_at) }), ticks));
    el.append(b);
    if (m.reactions.length) {
      el.append(h("div", { class: "reactions" }, m.reactions.map((r) => h("button", { class: "reaction" + (r.mine ? " mine" : ""), type: "button", "aria-pressed": r.mine ? "true" : "false", "aria-label": `${r.emoji} ${r.count}`, onclick: () => toggleReaction(m, r.emoji) }, r.emoji, h("small", { text: r.count })))));
    }
    return el;
  }
  function repaint(m) {
    const key = keyOf(m), old = cs.els.get(key);
    const el = msgEl(m); cs.els.set(key, el);
    if (old) old.replaceWith(el);
  }
  function refreshTicks() {
    for (const m of cs.msgs) if (m.sender_id === S.me.id && !isPending(m) && cs.els.has(m.id)) repaint(m);
  }
  function jumpTo(id) {
    const el = cs?.els.get(id);
    if (!el) { toast("Это сообщение находится выше в истории — прокрутите вверх"); return; }
    el.scrollIntoView({ block: "center" });
    el.classList.add("flash"); setTimeout(() => el.classList.remove("flash"), 1300);
  }

  /* add / replace / remove messages in the open chat */
  function addMessage(m) {
    if (cs.els.has(keyOf(m)) || cs.msgs.some((x) => x.id === m.id && m.id != null)) return false;
    const stick = nearBottom() || m.sender_id === S.me.id;
    const wrap = $("#wrap");
    const pendIdx = isPending(m) ? -1 : cs.msgs.findIndex(isPending);
    const empty = wrap.querySelector(".empty"); if (empty) empty.remove();
    const prev = pendIdx >= 0 ? cs.msgs[pendIdx - 1] : cs.msgs[cs.msgs.length - 1];
    const d = new Date(m.created_at);
    const needDay = !prev || !sameDay(new Date(prev.created_at), d);
    const el = msgEl(m);
    cs.els.set(keyOf(m), el);
    if (pendIdx >= 0) {
      const anchor = cs.els.get(keyOf(cs.msgs[pendIdx]));
      cs.msgs.splice(pendIdx, 0, m);
      if (needDay) wrap.insertBefore(h("div", { class: "day", text: dayLabel(d) }), anchor);
      wrap.insertBefore(el, anchor);
    } else {
      cs.msgs.push(m);
      if (needDay) wrap.append(h("div", { class: "day", text: dayLabel(d) }));
      wrap.append(el);
    }
    if (stick) scrollBottom(); else { cs.unseen++; paintJump(); }
    return true;
  }
  function removeMessage(id) {
    const i = cs.msgs.findIndex((x) => x.id === id); if (i < 0) return;
    cs.msgs.splice(i, 1);
    const el = cs.els.get(id); cs.els.delete(id);
    if (el) { const prev = el.previousElementSibling, next = el.nextElementSibling; el.remove(); if (prev?.classList.contains("day") && (!next || next.classList.contains("day"))) prev.remove(); }
    cs.pins = cs.pins.filter((p) => p.id !== id); drawPin();
  }
  function confirmPending(pm, m) {
    normalize(m);
    const i = cs.msgs.indexOf(pm); if (i < 0) return;
    const pk = keyOf(pm), pel = cs.els.get(pk); cs.els.delete(pk);
    if (cs.els.has(m.id)) { cs.msgs.splice(i, 1); pel?.remove(); return; } // the WebSocket event got there first
    cs.msgs[i] = m;
    const el = msgEl(m); cs.els.set(m.id, el);
    pel ? pel.replaceWith(el) : $("#wrap").append(el);
  }

  /* sending */
  function submitComposer() {
    if (!cs || !cs.loaded) return;
    const ta = $("#ta"), text = ta.value.trim();
    if (!text) return;
    if (cs.editing) { doEdit(cs.editing, text); return; }
    const reply = cs.replyTo;
    ta.value = ""; ta.dispatchEvent(new Event("input")); drafts.delete(cs.id);
    clearContext(false);
    sendText(text, reply);
  }
  function sendText(text, reply) {
    const pm = { id: null, client_id: uuid(), chat_id: cs.id, sender_id: S.me.id, sender_username: S.me.username, body: text, created_at: new Date().toISOString(),
      reply_to_message_id: reply ? reply.id : undefined, reply_to_body: reply ? reply.body : undefined, reply_to_sender_username: reply ? reply.sender_username : undefined,
      reactions: [], status: "sending" };
    addMessage(pm);
    transmit(pm);
  }
  async function transmit(pm) {
    const chat = cs; if (!chat || chat.id !== pm.chat_id) return;
    if (pm.status === "failed") { pm.status = "sending"; repaint(pm); }
    try {
      const body = { text: pm.body, client_id: pm.client_id };
      if (pm.reply_to_message_id) body.reply_to_message_id = pm.reply_to_message_id;
      const m = await api(`/chats/${pm.chat_id}/messages`, { method: "POST", body });
      if (cs === chat) confirmPending(pm, m);
    } catch (e) {
      if (cs !== chat) return;
      pm.status = "failed"; repaint(pm);
      toast(e.status === 0 ? "Не отправлено: нет соединения. Нажмите ⋯, чтобы повторить" : errText(e), { error: true });
    }
  }
  async function doEdit(m, text) {
    if (text === m.body) { clearContext(true); return; }
    try {
      const upd = normalize(await api(`/chats/${cs.id}/messages/${m.id}`, { method: "PATCH", body: { text } }));
      clearContext(true);
      applyEdit(upd.id, upd.body, upd.edited_at);
    } catch (e) { fail(e); }
  }
  function applyEdit(id, body, editedAt) {
    const m = cs.msgs.find((x) => x.id === id);
    if (m) { m.body = body; m.edited_at = editedAt; repaint(m); }
    for (const r of cs.msgs) if (r.reply_to_message_id === id) { r.reply_to_body = body; repaint(r); }
    const p = cs.pins.find((x) => x.id === id); if (p) { p.body = body; drawPin(); }
  }

  /* reply / edit context bar */
  function setContext(kind, m) {
    cs.replyTo = kind === "reply" ? m : null; cs.editing = kind === "edit" ? m : null;
    const host = $("#ctxhost");
    host.replaceChildren(h("div", { class: "ctx" },
      h("div", { class: "ctx-body" }, h("b", { text: kind === "reply" ? "Ответ @" + m.sender_username : "Редактирование" }), h("span", { text: m.body })),
      iconBtn("x", "Отменить", () => clearContext(true))));
    const ta = $("#ta");
    if (kind === "edit") { ta.value = m.body; ta.dispatchEvent(new Event("input")); }
    ta.focus();
  }
  function clearContext(resetText) {
    if (!cs) return;
    const wasEdit = !!cs.editing; cs.replyTo = null; cs.editing = null;
    $("#ctxhost")?.replaceChildren();
    if (resetText && wasEdit) { const ta = $("#ta"); ta.value = ""; ta.dispatchEvent(new Event("input")); }
  }

  /* message actions menu */
  function openMenu(m) {
    const mine = m.sender_id === S.me.id;
    const d = h("dialog");
    const close = () => d.close();
    const item = (label, fn, cls = "") => h("button", { class: "menu-item " + cls, type: "button", onclick: () => { close(); fn(); } }, label);
    let body;
    if (isPending(m)) {
      body = [m.status === "failed" && item("Повторить отправку", () => transmit(m)), item("Удалить", () => { const el = cs.els.get(keyOf(m)); cs.els.delete(keyOf(m)); el?.remove(); cs.msgs.splice(cs.msgs.indexOf(m), 1); }, "danger")];
    } else {
      const mineSet = new Set(m.reactions.filter((r) => r.mine).map((r) => r.emoji));
      body = [
        h("div", { class: "emoji-row" }, REACTIONS.map((e) => h("button", { type: "button", class: mineSet.has(e) ? "on" : "", "aria-label": "Реакция " + e, onclick: () => { close(); toggleReaction(m, e, mineSet.has(e)); } }, e))),
        item("Ответить", () => setContext("reply", m)),
        item("Копировать текст", () => copyText(m.body)),
        item("Переслать", () => openForward(m)),
        item(m.pinned ? "Открепить" : "Закрепить", () => togglePin(m)),
        mine && item("Изменить", () => setContext("edit", m)),
        mine && item("Удалить", () => confirmDelete(m), "danger"),
      ];
    }
    d.append(h("div", { class: "dlg" }, h("div", { class: "dlg-body" }, body)));
    d.addEventListener("click", (e) => { if (e.target === d) d.close(); });
    d.addEventListener("close", () => d.remove());
    document.body.append(d); d.showModal();
  }
  async function copyText(t) {
    try { await navigator.clipboard.writeText(t); toast("Скопировано"); } catch { toast("Не удалось скопировать", { error: true }); }
  }
  function confirmDelete(m) {
    const d = dialog("Удалить сообщение?", h("div", { class: "muted", text: "Сообщение исчезнет у всех участников чата." }),
      h("div", { class: "actions" }, h("button", { class: "btn ghost", type: "button", text: "Отмена", onclick: () => d.close() }),
        h("button", { class: "btn danger", type: "button", text: "Удалить", onclick: async () => {
          d.close();
          try { await api(`/chats/${cs.id}/messages/${m.id}`, { method: "DELETE" }); if (cs && cs.msgs.includes(m)) removeMessage(m.id); loadChatsSoon(); } catch (e) { fail(e); }
        } })));
  }
  async function toggleReaction(m, emoji, has) {
    if (has === undefined) has = m.reactions.some((r) => r.emoji === emoji && r.mine);
    const chatId = cs.id;
    try {
      const res = has
        ? await api(`/chats/${chatId}/messages/${m.id}/reactions?emoji=${encodeURIComponent(emoji)}`, { method: "DELETE" })
        : await api(`/chats/${chatId}/messages/${m.id}/reactions`, { method: "POST", body: { emoji } });
      if (cs && cs.id === chatId) applyReaction(res.message_id, res.counts, res.actor_emojis, true);
    } catch (e) { fail(e); }
  }
  function applyReaction(id, counts, actorEmojis, actorIsMe) {
    const m = cs.msgs.find((x) => x.id === id); if (!m) return;
    const prevMine = new Set(m.reactions.filter((r) => r.mine).map((r) => r.emoji));
    m.reactions = (counts || []).map((c) => ({ emoji: c.emoji, count: c.count, mine: actorIsMe ? (actorEmojis || []).includes(c.emoji) : prevMine.has(c.emoji) }));
    repaint(m);
  }
  async function togglePin(m) {
    try {
      await api(`/chats/${cs.id}/pins/${m.id}`, { method: m.pinned ? "DELETE" : "PUT" });
      m.pinned = !m.pinned; repaint(m); await reloadPins();
    } catch (e) { fail(e); }
  }
  async function reloadPins() {
    const chat = cs; if (!chat) return;
    try { const { messages } = await api(`/chats/${chat.id}/pins`); if (cs === chat) { chat.pins = messages || []; chat.pinIdx = 0; for (const m of chat.msgs) { const p = !!chat.pins.find((x) => x.id === m.id); if (m.pinned !== p) { m.pinned = p; repaint(m); } } drawPin(); } } catch { /* shown on next open */ }
  }
  function drawPin() {
    const host = $("#pinhost"); if (!host || !cs) return;
    if (!cs.pins.length) { host.replaceChildren(); return; }
    const p = cs.pins[cs.pinIdx % cs.pins.length];
    host.replaceChildren(h("button", { class: "pinbar", type: "button", onclick: () => { jumpTo(p.id); cs.pinIdx = (cs.pinIdx + 1) % cs.pins.length; drawPin(); } },
      h("span", null, h("span", { class: "pin-title", text: cs.pins.length > 1 ? `Закреплённое сообщение ${cs.pinIdx % cs.pins.length + 1}/${cs.pins.length}` : "Закреплённое сообщение" }), h("span", { class: "pin-text", text: p.body }))));
  }
  function openForward(m) {
    const targets = S.chats;
    const d = dialog("Переслать в…", ...(targets.length ? targets.map((c) => h("button", { class: "row", type: "button", onclick: async () => {
      d.close();
      try {
        const sent = normalize(await api(`/chats/${cs.id}/messages/${m.id}/forward`, { method: "POST", body: { to_chat_id: c.id, client_id: uuid() } }));
        toast("Переслано: " + chatName(c));
        if (cs && cs.id === c.id) addMessage(sent);
        loadChatsSoon();
      } catch (e) { fail(e); }
    } }, avatar(peerOf(c), 36), h("div", { class: "row-body" }, h("div", { class: "row-name", text: chatName(c) })))) : [h("div", { class: "empty", text: "Нет чатов, куда можно переслать" })]));
  }

  /* reading */
  let readTimer = 0;
  function scheduleRead() { clearTimeout(readTimer); readTimer = setTimeout(markReadNow, 250); }
  function markReadNow() {
    if (!cs || !cs.loaded || document.hidden || !nearBottom()) return;
    const last = lastRealId();
    if (!last || last <= cs.myRead) return;
    cs.myRead = last;
    const c = chatById(cs.id); if (c && c.unread_count) { c.unread_count = 0; renderList(); updateTitle(); }
    api(`/chats/${cs.id}/read`, { method: "POST", body: { message_id: last } }).catch(() => {});
  }

  /* ---------------- realtime ---------------- */
  const RT = { ws: null, tries: 0, timer: 0, ready: false, stopped: true, everReady: false, poll: 0 };
  function rtStart() { RT.stopped = false; rtConnect(); clearInterval(RT.poll); RT.poll = setInterval(rtPoll, 60000); }
  function rtStop() {
    RT.stopped = true; clearTimeout(RT.timer); clearInterval(RT.poll);
    if (RT.ws) { RT.ws.onclose = null; try { RT.ws.close(1000); } catch { /* ignore */ } RT.ws = null; }
    RT.ready = false; RT.everReady = false;
  }
  function rtSchedule() {
    if (RT.stopped) return;
    const delay = Math.min(30000, 1000 * 2 ** RT.tries++) * (0.6 + Math.random() * 0.4);
    clearTimeout(RT.timer); RT.timer = setTimeout(rtConnect, delay);
  }
  async function rtConnect() {
    if (RT.stopped || RT.ws) return;
    const t = loadTokens(); if (!t) return;
    try {
      // /me refreshes the access token transparently if it has expired.
      if (t.exp - Date.now() < 60000 || RT.tries > 0) await api("/me");
    } catch (e) { if (e.status === 401) return; rtSchedule(); return; }
    if (RT.stopped || RT.ws) return;
    const ws = new WebSocket((location.protocol === "https:" ? "wss://" : "ws://") + location.host + API + "/ws");
    RT.ws = ws; RT.ready = false;
    ws.onopen = () => { const tk = loadTokens(); if (tk) ws.send(JSON.stringify({ type: "auth", token: tk.access })); };
    ws.onmessage = (e) => {
      let ev; try { ev = JSON.parse(e.data); } catch { return; }
      if (ev.type === "ready") { RT.ready = true; RT.tries = 0; onReady(); return; }
      onRT(ev);
    };
    ws.onclose = () => { if (RT.ws === ws) RT.ws = null; RT.ready = false; showNet(!navigator.onLine || RT.everReady); rtSchedule(); };
    ws.onerror = () => { try { ws.close(); } catch { /* ignore */ } };
  }
  async function onReady() {
    const again = RT.everReady; RT.everReady = true; showNet(false);
    try { const { online } = await api("/presence"); S.online = new Set(online || []); updateDots(); paintStatus(); } catch { /* next poll */ }
    if (again) { await loadChats(); if (cs && cs.loaded) resyncActive(); } // anything missed while offline
  }
  async function rtPoll() {
    if (!RT.ready) return;
    try { const { online } = await api("/presence"); S.online = new Set(online || []); updateDots(); paintStatus(); }
    catch (e) { if (e.status === 0 && RT.ws) RT.ws.close(); } // network is gone though the socket looks open
  }
  async function resyncActive() {
    const chat = cs;
    try {
      const hist = await api(`/chats/${chat.id}/messages`);
      if (cs !== chat) return;
      const pend = chat.msgs.filter(isPending);
      chat.msgs = (hist.messages || []).map(normalize).concat(pend);
      chat.peerRead = hist.peer_last_read_message_id || chat.peerRead;
      chat.hasMore = hist.messages.length >= PAGE; chat.myRead = Math.max(chat.myRead, lastRealId());
      const stick = nearBottom(); drawMessages(); if (stick) scrollBottom(); await reloadPins();
    } catch { /* the next reconnect retries */ }
  }
  let netBar = null;
  function showNet(on) {
    if (on && !netBar) { netBar = h("div", { class: "net", text: "Нет соединения. Переподключаемся…" }); $("#app").prepend(netBar); }
    if (!on && netBar) { netBar.remove(); netBar = null; }
  }
  function updateDots() {
    document.querySelectorAll(".avatar[data-uid]").forEach((el) => { const id = +el.dataset.uid; if (id) el.classList.toggle("online", S.online.has(id)); });
  }
  function setTyping(chatId) {
    const prev = S.typing.get(chatId); if (prev) clearTimeout(prev);
    S.typing.set(chatId, setTimeout(() => { S.typing.delete(chatId); renderList(); if (cs && cs.id === chatId) paintStatus(); }, 4000));
    renderList(); if (cs && cs.id === chatId) paintStatus();
  }

  function onRT(ev) {
    const c = ev.chat_id ? chatById(ev.chat_id) : null;
    switch (ev.type) {
      case "presence":
        if (ev.data.online) S.online.add(ev.data.user_id); else S.online.delete(ev.data.user_id);
        updateDots(); paintStatus(); return;
      case "typing":
        if (ev.data.user_id !== S.me.id) setTyping(ev.chat_id);
        return;
      case "message.new": {
        const m = ev.data, mine = m.sender_id === S.me.id;
        if (!c) loadChats();
        else {
          c.last_message = m.body; c.last_message_at = m.created_at;
          const watching = cs && cs.id === c.id && !document.hidden && nearBottom();
          if (!mine && !watching) c.unread_count = (c.unread_count || 0) + 1;
          S.chats = [c, ...S.chats.filter((x) => x !== c)];
          S.typing.delete(c.id);
          renderList(); updateTitle();
        }
        break;
      }
      case "message.edited": case "message.deleted": loadChatsSoon(); break;
      case "chat.read":
        if (ev.data.user_id === S.me.id && c) { c.unread_count = 0; renderList(); updateTitle(); }
        break;
      default: break;
    }
    if (cs && ev.chat_id === cs.id) { if (!cs.loaded) cs.queue.push(ev); else handleChatEvent(ev); }
  }
  function handleChatEvent(ev) {
    const d = ev.data;
    switch (ev.type) {
      case "message.new": {
        const m = normalize(d);
        if (addMessage(m) && m.sender_id !== S.me.id) scheduleRead();
        break;
      }
      case "message.edited": applyEdit(d.id, d.body, d.edited_at); break;
      case "message.deleted": removeMessage(d.message_id); break;
      case "reaction.updated": applyReaction(d.message_id, d.counts, d.actor_emojis, d.actor_id === S.me.id); break;
      case "message.pinned": case "message.unpinned": reloadPins(); break;
      case "chat.read":
        if (d.user_id !== S.me.id && d.last_read_message_id > cs.peerRead) { cs.peerRead = d.last_read_message_id; refreshTicks(); }
        break;
      default: break;
    }
  }

  /* ---------------- lifecycle ---------------- */
  function signOut() {
    clearTokens(); rtStop();
    S.me = null; S.chats = []; S.contacts = null; S.online = new Set(); cs = null; S.active = null; drafts.clear();
    document.title = "KentGram"; netBar = null;
    mountAuth();
  }
  async function startApp() {
    mountApp();
    history.replaceState({ kg: 0 }, "", location.hash || "#/");
    await loadChats();
    route();
    rtStart();
  }

  function fitViewport() {
    const vv = window.visualViewport, app = $("#app"); if (!vv || !app) return;
    app.style.height = vv.height + "px"; app.style.top = vv.offsetTop + "px";
    if (cs && nearBottomBefore) scrollBottom();
  }
  let nearBottomBefore = false;

  function registerSW() {
    if (!("serviceWorker" in navigator)) return;
    const hadController = !!navigator.serviceWorker.controller;
    navigator.serviceWorker.register("/sw.js").catch(() => {});
    if (hadController) {
      navigator.serviceWorker.addEventListener("controllerchange", () => {
        toast("Вышла новая версия KentGram", { sticky: true, action: { label: "Обновить", run: () => location.reload() } });
      });
    }
  }

  async function boot() {
    registerSW();
    window.addEventListener("popstate", route);
    window.addEventListener("online", () => { if (S.me && !RT.ws) { RT.tries = 0; clearTimeout(RT.timer); rtConnect(); } });
    window.addEventListener("offline", () => S.me && showNet(true));
    document.addEventListener("visibilitychange", () => { if (!document.hidden) { markReadNow(); if (S.me && !RT.ws && !RT.stopped) { RT.tries = 0; clearTimeout(RT.timer); rtConnect(); } } });
    window.addEventListener("storage", (e) => { if (e.key === TK && !e.newValue && S.me) signOut(); });
    if (window.visualViewport) {
      window.visualViewport.addEventListener("resize", () => { nearBottomBefore = cs ? nearBottom() : false; fitViewport(); });
      window.visualViewport.addEventListener("scroll", fitViewport);
    }
    if (!loadTokens()) { mountAuth(); return; }
    try { S.me = await api("/me"); } catch (e) {
      if (e.status === 401 || !loadTokens()) { mountAuth(); return; }
      $("#app").replaceChildren(h("div", { class: "placeholder" }, h("div", null, h("div", { text: errText(e) }), h("p", null, h("button", { class: "btn", type: "button", text: "Повторить", onclick: () => location.reload() })))));
      return;
    }
    await startApp();
  }
  boot();
})();
