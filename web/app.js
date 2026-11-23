(() => {
  "use strict";
  const API = "https://api.kentgram.tendercheck.pro/api/v1";
  const WS = "wss://api.kentgram.tendercheck.pro/api/v1/realtime";
  const $ = (id) => document.getElementById(id);
  const state = {
    access: sessionStorage.getItem("kg_access") || "",
    refresh: sessionStorage.getItem("kg_refresh") || "",
    me: null, chats: [], chat: null, replyToMessageId: 0, socket: null, socketTimer: 0,
    socketDelay: 1000, authMode: "login", searchTimer: 0, toastTimer: 0,
    refreshing: null, booting: false, stopped: false, messageRequest: 0,
    realtimeCursor: 0, realtimeSyncing: false
  };
  const MAX_FILE = 20 * 1024 * 1024;
  const ALLOWED_TYPES = new Set(["image/jpeg", "image/png", "application/pdf"]);

  function toast(message) {
    const el = $("toast");
    if (!el) return;
    el.textContent = message;
    el.classList.add("show");
    clearTimeout(state.toastTimer);
    state.toastTimer = setTimeout(() => el.classList.remove("show"), 3000);
  }

  function saveTokens(data) {
    if (!data?.access_token || !data?.refresh_token) throw new Error("invalid_auth_response");
    state.access = data.access_token;
    state.refresh = data.refresh_token;
    sessionStorage.setItem("kg_access", state.access);
    sessionStorage.setItem("kg_refresh", state.refresh);
  }

  function clearTokens() {
    state.access = "";
    state.refresh = "";
    sessionStorage.removeItem("kg_access");
    sessionStorage.removeItem("kg_refresh");
  }

  function translateError(code) {
    const messages = {
      invalid_credentials: "Неверное имя пользователя или пароль.",
      username_or_email_taken: "Имя пользователя или почта уже заняты.",
      invalid_registration_data: "Проверь имя пользователя, почту и пароль (не менее 10 символов).",
      too_many_login_attempts: "Слишком много попыток. Попробуй позже.",
      rate_limit_exceeded: "Слишком много запросов. Подожди минуту и попробуй снова.",
      origin_not_allowed: "Источник запроса не разрешён.",
      invalid_auth_response: "Сервер вернул неполный ответ авторизации.",
      media_request_failed: "Не удалось загрузить вложение.",
      request_failed: "Не удалось выполнить запрос."
    };
    return messages[code] || "Не удалось выполнить запрос: " + code;
  }

  async function request(path, options = {}) {
    const headers = new Headers(options.headers || {});
    if (options.auth !== false && state.access) headers.set("Authorization", "Bearer " + state.access);
    let response;
    try {
      response = await fetch(API + path, { ...options, headers, cache: "no-store" });
    } catch {
      throw new Error("Не удалось связаться с сервером. Проверь подключение.");
    }
    const type = response.headers.get("content-type") || "";
    const data = type.includes("application/json") ? await response.json().catch(() => ({})) : {};
    if (!response.ok) {
      const error = new Error(data.error || "request_failed");
      error.status = response.status;
      throw error;
    }
    return data;
  }

  async function refreshTokens() {
    if (!state.refresh) throw new Error("session_expired");
    if (!state.refreshing) {
      state.refreshing = request("/auth/refresh", {
        method: "POST", auth: false,
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ refresh_token: state.refresh })
      }).then(saveTokens).finally(() => { state.refreshing = null; });
    }
    return state.refreshing;
  }

  async function api(path, options = {}) {
    try {
      return await request(path, options);
    } catch (error) {
      if (error.status !== 401 || options.auth === false || !state.refresh || path === "/auth/refresh") throw error;
      await refreshTokens();
      return request(path, options);
    }
  }

  async function protectedFetch(url) {
    const send = () => fetch(url, { headers: { Authorization: "Bearer " + state.access }, cache: "no-store" });
    let response = await send();
    if (response.status === 401 && state.refresh) {
      await refreshTokens();
      response = await send();
    }
    if (!response.ok) {
      const error = new Error("media_request_failed");
      error.status = response.status;
      throw error;
    }
    return response;
  }

  function node(tag, className, value) {
    const el = document.createElement(tag);
    if (className) el.className = className;
    if (value !== undefined) el.textContent = String(value);
    return el;
  }

  function showAuth(message = "") {
    stopSocket();
    $("app").classList.add("hidden");
    $("auth").classList.remove("hidden");
    $("auth-notice").textContent = message;
  }

  function showApp() {
    $("auth").classList.add("hidden");
    $("app").classList.remove("hidden");
    $("my-name").textContent = state.me.display_name || state.me.username;
    $("my-handle").textContent = "@" + state.me.username;
    $("my-avatar").textContent = (state.me.display_name || state.me.username || "K").slice(0, 1).toUpperCase();
  }

  function setAuthMode(mode) {
    state.authMode = mode;
    const register = mode === "register";
    $("auth-title").textContent = register ? "Создать аккаунт" : "С возвращением";
    $("auth-subtitle").textContent = register ? "Зарегистрируй kentGram аккаунт." : "Войди, чтобы продолжить общение.";
    $("email-field").classList.toggle("hidden", !register);
    $("auth-email").required = register;
    $("auth-password").autocomplete = register ? "new-password" : "current-password";
    $("auth-password").minLength = register ? 10 : 0;
    $("auth-submit").textContent = register ? "Зарегистрироваться" : "Войти";
    $("auth-toggle").textContent = register ? "Уже есть аккаунт? Войти" : "Создать аккаунт";
    $("auth-notice").textContent = "";
  }

  $("auth-toggle").addEventListener("click", () => setAuthMode(state.authMode === "login" ? "register" : "login"));

  $("auth-form").addEventListener("submit", async (event) => {
    event.preventDefault();
    const button = $("auth-submit");
    if (button.disabled) return;
    button.disabled = true;
    $("auth-notice").textContent = "";
    try {
      const registering = state.authMode === "register";
      const body = {
        username: $("auth-username").value.trim().replace(/^@+/, ""),
        password: $("auth-password").value,
        device_name: "kentGram Web"
      };
      if (registering) body.email = $("auth-email").value.trim();
      const data = await request(registering ? "/auth/register" : "/auth/login", {
        method: "POST", auth: false,
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body)
      });
      saveTokens(data);
      state.me = data.user || await api("/me");
      state.realtimeCursor = Number(sessionStorage.getItem("kg_realtime_cursor_" + state.me.id) || 0);
      await boot();
    } catch (error) {
      $("auth-notice").textContent = translateError(error.message);
    } finally {
      button.disabled = false;
    }
  });

  function chatName(chat) {
    return chat.kind === "direct"
      ? (chat.peer_display_name || chat.peer_username || "Диалог")
      : (chat.title || chat.username || "Сообщество");
  }

  function avatarText(chat) {
    return chatName(chat).slice(0, 1).toUpperCase() || "K";
  }

  async function loadChats() {
    const data = await api("/chats");
    state.chats = Array.isArray(data.chats) ? data.chats : [];
    const list = $("chat-list");
    list.replaceChildren();
    if (!state.chats.length) {
      list.append(node("p", "muted chat-empty", "Пока нет чатов. Найди человека через поиск выше."));
      return;
    }
    const fragment = document.createDocumentFragment();
    for (const chat of state.chats) {
      const item = node("button", "chat-item" + (state.chat?.id === chat.id ? " active" : ""));
      item.type = "button";
      item.setAttribute("aria-current", state.chat?.id === chat.id ? "page" : "false");
      item.append(node("div", "avatar", avatarText(chat)));
      const preview = node("div", "chat-preview");
      preview.append(
        node("strong", "", chatName(chat)),
        node("span", "", chat.last_message || chat.description || "Начни общение")
      );
      item.append(preview);
      if (Number(chat.unread_count) > 0) item.append(node("span", "unread", chat.unread_count));
      item.addEventListener("click", () => openChat(chat.id));
      fragment.append(item);
    }
    list.append(fragment);
  }

  function scrollMessagesToBottom(area) {
    requestAnimationFrame(() => { area.scrollTop = area.scrollHeight; });
  }

  function renderMessage(message) {
    const box = node("article", "message" + (message.is_mine ? " mine" : ""));
    if (!message.is_mine) box.append(node("span", "sender", message.sender_username || "Пользователь"));
    if (message.forwarded_from_username) {
      box.append(node("span", "sender", "Переслано от @" + message.forwarded_from_username));
    }
    if (message.media_id && message.media_url) {
      const url = API + message.media_url;

      if ((message.media_mime || "").startsWith("image/")) {
        const media = node("div", "message-media message-media-image");
        const placeholder = node("div", "media-placeholder", "Загрузка изображения…");
        const img = node("img");
        img.alt = message.media_name || "Изображение";
        img.loading = "lazy";
        img.decoding = "async";

        media.append(placeholder, img);
        box.append(media);

        let objectURL = null;

        const showError = () => {
          if (objectURL) {
            URL.revokeObjectURL(objectURL);
            objectURL = null;
          }
          img.remove();
          placeholder.classList.add("media-error");
          placeholder.textContent = "Не удалось загрузить изображение";
        };

        protectedFetch(url)
          .then(async (response) => {
            const blob = await response.blob();
            if (!blob.size || !blob.type.startsWith("image/")) {
              throw new Error("invalid_image");
            }
            return blob;
          })
          .then((blob) => {
            if (!media.isConnected) return;
            objectURL = URL.createObjectURL(blob);

            img.addEventListener("load", () => {
              placeholder.remove();
              media.classList.add("media-loaded");
              if (objectURL) {
                URL.revokeObjectURL(objectURL);
                objectURL = null;
              }
            }, { once: true });

            img.addEventListener("error", showError, { once: true });
            img.src = objectURL;
          })
          .catch(() => {
            if (media.isConnected) showError();
          });
      } else {
        const link = node("a", "body", message.media_name || "Скачать вложение");
        link.href = "#";
        link.addEventListener("click", async (event) => {
          event.preventDefault();
          try {
            const response = await protectedFetch(url);
            const blob = await response.blob();
            const objectURL = URL.createObjectURL(blob);
            const anchor = document.createElement("a");
            anchor.href = objectURL;
            anchor.download = message.media_name || "attachment";
            document.body.append(anchor);
            anchor.click();
            anchor.remove();
            setTimeout(() => URL.revokeObjectURL(objectURL), 1500);
          } catch (error) {
            toast(translateError(error.message));
          }
        });
        box.append(link);
      }
    }
    if (message.reply_to_message_id) {
      box.append(node("div", "reply-reference", "Ответ на @" +
        (message.reply_to_sender_username || "пользователя") + ": " +
        (message.reply_to_body || "сообщение недоступно")));
    }
    if (message.body) box.append(node("div", "body", message.body));
    if (message.edited_at) box.append(node("small", "edited-label", "изменено"));
    const reactions = node("div", "message-reactions");
    for (const reaction of (message.reactions || [])) {
      const chip = node("button", "reaction-chip" + (reaction.reacted ? " selected" : ""),
        reaction.emoji + " " + reaction.count);
      chip.type = "button";
      chip.addEventListener("click", async () => {
        try {
          await api("/chats/" + encodeURIComponent(message.chat_id) + "/messages/" +
            encodeURIComponent(message.id) + "/reactions", {
              method: "POST", headers: { "Content-Type": "application/json" },
              body: JSON.stringify({ emoji: reaction.emoji })
            });
          if (state.chat?.id === message.chat_id) await loadMessages(message.chat_id);
        } catch (error) { toast(translateError(error.message)); }
      });
      reactions.append(chip);
    }
    if (reactions.childElementCount) box.append(reactions);

    const actions = node("div", "message-actions");
    const replyButton = node("button", "message-action", "Ответить");
    replyButton.type = "button";
    replyButton.addEventListener("click", () => {
      state.replyToMessageId = message.id;
      $("message-input").placeholder = "Ответ на сообщение…";
      $("message-input").focus();
      toast("Ответ на сообщение #" + message.id);
    });
    actions.append(replyButton);

    const reactButton = node("button", "message-action", "Реакция");
    reactButton.type = "button";
    reactButton.addEventListener("click", async () => {
      const emoji = prompt("Введи эмодзи для реакции:");
      if (!emoji) return;
      try {
        await api("/chats/" + encodeURIComponent(message.chat_id) + "/messages/" +
          encodeURIComponent(message.id) + "/reactions", {
            method: "POST", headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ emoji })
          });
        if (state.chat?.id === message.chat_id) await loadMessages(message.chat_id);
      } catch (error) { toast(translateError(error.message)); }
    });
    actions.append(reactButton);

    if (message.is_mine) {
      const editButton = node("button", "message-action", "Изменить");
      editButton.type = "button";
      editButton.addEventListener("click", async () => {
        const text = prompt("Измени сообщение:", message.body || "");
        if (text === null || !text.trim()) return;
        try {
          await api("/chats/" + encodeURIComponent(message.chat_id) + "/messages/" +
            encodeURIComponent(message.id), {
              method: "PATCH", headers: { "Content-Type": "application/json" },
              body: JSON.stringify({ text })
            });
          await loadMessages(message.chat_id);
        } catch (error) { toast(translateError(error.message)); }
      });
      actions.append(editButton);

      const deleteButton = node("button", "message-action danger", "Удалить");
      deleteButton.type = "button";
      deleteButton.addEventListener("click", async () => {
        if (!confirm("Удалить это сообщение?")) return;
        try {
          await api("/chats/" + encodeURIComponent(message.chat_id) + "/messages/" +
            encodeURIComponent(message.id), { method: "DELETE" });
          await loadMessages(message.chat_id);
        } catch (error) { toast(translateError(error.message)); }
      });
      actions.append(deleteButton);
    }
    box.append(actions);

    const date = new Date(message.created_at);
    box.append(node("time", "", Number.isNaN(date.getTime())
      ? "" : date.toLocaleString("ru-RU", { hour: "2-digit", minute: "2-digit" })));
    return box;
  }

  async function loadMessages(chatId) {
    const requestId = ++state.messageRequest;
    const data = await api("/chats/" + encodeURIComponent(chatId) + "/messages");
    if (!state.chat || state.chat.id !== chatId || requestId !== state.messageRequest) return;
    const area = $("messages");
    const nearBottom = area.scrollHeight - area.scrollTop - area.clientHeight < 100;
    const fragment = document.createDocumentFragment();
    for (const message of (Array.isArray(data.messages) ? data.messages : [])) fragment.append(renderMessage(message));
    area.replaceChildren(fragment);
    if (nearBottom || requestId === 1) scrollMessagesToBottom(area);

    const messages = Array.isArray(data.messages) ? data.messages : [];
    const latest = messages[messages.length - 1];
    if (latest) {
      api("/chats/" + encodeURIComponent(chatId) + "/read", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ last_read_message_id: latest.id })
      }).catch(() => {});
    }
  }

  async function openChat(id) {
    const chat = state.chats.find((item) => item.id === id);
    if (!chat) return;
    state.chat = chat;
    state.replyToMessageId = 0;
    $("message-input").placeholder = "Написать сообщение…";
    $("welcome").classList.add("hidden");
    $("chat-header").classList.remove("hidden");
    $("messages").classList.remove("hidden");
    $("composer").classList.remove("hidden");
    $("settings-panel").classList.add("hidden");
    $("app").classList.remove("settings-open");
    $("app").classList.add("chat-open");
    $("chat-title").textContent = chatName(chat);
    $("chat-subtitle").textContent = chat.kind === "direct"
      ? "@" + chat.peer_username : (chat.member_count + " участников");
    $("chat-avatar").textContent = avatarText(chat);
    try {
      await loadMessages(id);
      await loadChats();
      connectSocket();
    } catch (error) {
      toast(translateError(error.message));
    }
  }

  function closeChatView() {
    state.chat = null;
    state.replyToMessageId = 0;
    $("message-input").placeholder = "Написать сообщение…";
    state.messageRequest++;
    $("app").classList.remove("chat-open");
    $("welcome").classList.remove("hidden");
    $("chat-header").classList.add("hidden");
    $("messages").classList.add("hidden");
    $("composer").classList.add("hidden");
  }

  $("back-to-chats").addEventListener("click", closeChatView);

  $("message-input").addEventListener("keydown", (event) => {
    if (event.key === "Enter" && !event.shiftKey && !event.isComposing) {
      event.preventDefault();
      $("composer").requestSubmit();
    }
  });

  $("composer").addEventListener("submit", async (event) => {
    event.preventDefault();
    if (!state.chat) return;
    const chatId = state.chat.id;
    const input = $("message-input");
    const text = input.value.trim();
    if (!text) return;
    const button = $("composer").querySelector("button[type=submit]");
    input.value = "";
    button.disabled = true;
    try {
      await api("/chats/" + encodeURIComponent(chatId) + "/messages", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ text, reply_to_message_id: state.replyToMessageId || 0 })
      });
      state.replyToMessageId = 0;
      input.placeholder = "Написать сообщение…";
      if (state.chat?.id === chatId) await loadMessages(chatId);
      await loadChats();
    } catch (error) {
      input.value = text;
      toast(translateError(error.message));
    } finally {
      button.disabled = false;
      input.focus();
    }
  });

  $("attachment").addEventListener("change", async (event) => {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file || !state.chat) return;
    if (!ALLOWED_TYPES.has(file.type)) {
      toast("Поддерживаются только JPEG, PNG и PDF.");
      return;
    }
    if (file.size > MAX_FILE) {
      toast("Максимальный размер файла — 20 МБ.");
      return;
    }
    const chatId = state.chat.id;
    const form = new FormData();
    form.append("file", file);
    const caption = prompt("Подпись к файлу (необязательно):") || "";
    form.append("caption", caption);
    try {
      await api("/chats/" + encodeURIComponent(chatId) + "/media", { method: "POST", body: form });
      if (state.chat?.id === chatId) await loadMessages(chatId);
      await loadChats();
    } catch (error) {
      toast(translateError(error.message));
    }
  });

  $("people-search").addEventListener("input", (event) => {
    clearTimeout(state.searchTimer);
    const query = event.target.value.trim();
    if (query.length < 2) {
      $("search-results").classList.add("hidden");
      $("search-results").replaceChildren();
      return;
    }
    state.searchTimer = setTimeout(() => searchPeople(query), 300);
  });

  async function searchPeople(query) {
    try {
      const data = await api("/users/search?q=" + encodeURIComponent(query));
      if ($("people-search").value.trim() !== query) return;
      const box = $("search-results");
      box.replaceChildren();
      for (const person of (data.users || [])) {
        const button = node("button", "person-result");
        button.type = "button";
        const avatar = node("div", "avatar", (person.display_name || person.username || "?").slice(0, 1).toUpperCase());
        const info = node("span");
        info.append(node("strong", "", person.display_name || person.username), node("small", "", "@" + person.username));
        button.append(avatar, info);
        button.addEventListener("click", async () => {
          try {
            const result = await api("/chats/direct", {
              method: "POST",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify({ user_id: person.id })
            });
            $("people-search").value = "";
            box.classList.add("hidden");
            await loadChats();
            await openChat(result.chat_id);
          } catch (error) {
            toast(translateError(error.message));
          }
        });
        box.append(button);
      }
      if (!box.childElementCount) box.append(node("p", "muted", "Никого не нашли."));
      box.classList.remove("hidden");
    } catch (error) {
      toast(translateError(error.message));
    }
  }

  function stopSocket() {
    clearTimeout(state.socketTimer);
    state.socketTimer = 0;
    if (state.socket) {
      const socket = state.socket;
      state.socket = null;
      socket.onclose = null;
      socket.close();
    }
  }

  function scheduleSocketReconnect() {
    if (state.stopped || !state.access || !state.chat || state.socketTimer) return;
    const delay = state.socketDelay;
    state.socketDelay = Math.min(state.socketDelay * 2, 30000);
    state.socketTimer = setTimeout(() => {
      state.socketTimer = 0;
      connectSocket();
    }, delay);
  }

  function connectSocket() {
    if (!state.access || !state.chat || !window.WebSocket || state.stopped) return;
    if (state.socket && (state.socket.readyState === WebSocket.OPEN || state.socket.readyState === WebSocket.CONNECTING)) return;
    try {
      const socket = new WebSocket(WS, ["kentgram", "bearer." + state.access]);
      state.socket = socket;
      socket.onopen = () => {
        if (state.socket !== socket) return;
        state.socketDelay = 1000;
        syncRealtimeEvents();
        for (const chat of state.chats) {
          socket.send(JSON.stringify({ type: "presence.subscribe", chat_id: chat.id }));
        }
      };
      socket.onmessage = (event) => {
        let message;
        try { message = JSON.parse(event.data); } catch { return; }
        if (message.chat_id && state.chat && message.chat_id === state.chat.id &&
          ["message.created", "message.edited", "message.deleted", "message.reactions_updated", "chat.read", "poll.created", "poll.updated", "poll.closed"].includes(message.type)) {
          loadMessages(state.chat.id).catch(() => {});
          loadChats().catch(() => {});
        }
      };
      socket.onerror = () => socket.close();
      socket.onclose = () => {
        if (state.socket === socket) state.socket = null;
        scheduleSocketReconnect();
      };
    } catch {
      state.socket = null;
      scheduleSocketReconnect();
    }
  }





  async function syncRealtimeEvents() {
    if (!state.access || state.stopped || state.realtimeSyncing) return;
    state.realtimeSyncing = true;
    try {
      const data = await api("/realtime/events?since=" + encodeURIComponent(state.realtimeCursor));
      const events = Array.isArray(data.events) ? data.events : [];
      let refreshChats = false;
      let refreshCurrentChat = false;
      for (const event of events) {
        const cursor = Number(event.id || 0);
        if (cursor > state.realtimeCursor) state.realtimeCursor = cursor;
        const chatId = Number(event.chat_id || 0);
        if (chatId && state.chat?.id === chatId) refreshCurrentChat = true;
        if (["chat.created", "message.created", "message.edited", "message.deleted",
          "message.reactions_updated", "chat.read", "chat.pinned", "chat.unpinned",
          "poll.created", "poll.updated", "poll.closed"].includes(event.type)) refreshChats = true;
      }
      const highWater = Number(data.cursor || data.high_water || 0);
      if (highWater > state.realtimeCursor) state.realtimeCursor = highWater;
      sessionStorage.setItem("kg_realtime_cursor_" + state.me.id, String(state.realtimeCursor));
      if (refreshCurrentChat && state.chat) await loadMessages(state.chat.id);
      if (refreshChats) await loadChats();
    } catch (error) {
      console.debug("kentGram realtime recovery:", error.message);
    } finally {
      state.realtimeSyncing = false;
    }
  }

  // Автоматическая синхронизация веб-клиента.
  let autoSyncBusy = false;
  let lastChatsSync = 0;

  async function autoSync() {
    if (!state.access || state.stopped || document.hidden || autoSyncBusy) return;

    autoSyncBusy = true;
    try {
      const now = Date.now();
      await syncRealtimeEvents();

      if (now - lastChatsSync >= 10000) {
        await loadChats();
        lastChatsSync = now;
      }

      if (state.chat && !document.hidden && !state.stopped) {
        await loadMessages(state.chat.id);
      }
    } catch (error) {
      // Временная потеря сети не должна прерывать автоматическую синхронизацию.
      console.debug("kentGram auto-sync:", error.message);
    } finally {
      autoSyncBusy = false;
    }
  }

  const autoSyncTimer = setInterval(autoSync, 5000);

  document.addEventListener("visibilitychange", () => {
    if (!document.hidden) {
      lastChatsSync = 0;
      autoSync();
    }
  });

  $("logout-button").addEventListener("click", async () => {
    state.stopped = true;
    stopSocket();
    try {
      await api("/auth/logout", { method: "POST", headers: { "Content-Type": "application/json" }, body: "{}" });
    } catch {}
    clearTokens();
    state.me = null;
    state.chat = null;
    state.chats = [];
    state.stopped = false;
    showAuth();
  });

  $("settings-button").addEventListener("click", openSettings);
  $("settings-back").addEventListener("click", () => {
    $("settings-panel").classList.add("hidden");
    $("app").classList.remove("settings-open");
    if (state.chat) $("app").classList.add("chat-open");
  });

  async function openSettings() {
    $("app").classList.remove("chat-open");
    $("app").classList.add("settings-open");
    $("settings-panel").classList.remove("hidden");
    const box = $("web-sessions");
    box.replaceChildren(node("p", "muted", "Загружаем…"));
    try {
      const data = await api("/sessions");
      box.replaceChildren();
      for (const session of (data.sessions || [])) {
        const row = node("div", "session-row");
        row.append(node("div", "avatar", session.current ? "✓" : "▣"));
        const info = node("div");
        const usedAt = new Date(session.last_used_at);
        info.append(
          node("strong", "", session.device_name + (session.current ? " · это устройство" : "")),
          node("small", "", (session.ip || "IP неизвестен") + " · " +
            (Number.isNaN(usedAt.getTime()) ? "" : usedAt.toLocaleString("ru-RU")))
        );
        row.append(info);
        if (!session.current) {
          const button = node("button", "quiet-button danger", "Завершить");
          button.type = "button";
          button.addEventListener("click", async () => {
            try {
              await api("/sessions/" + encodeURIComponent(session.id), { method: "DELETE" });
              await openSettings();
            } catch (error) {
              toast(translateError(error.message));
            }
          });
          row.append(button);
        }
        box.append(row);
      }
      if (!box.childElementCount) box.append(node("p", "muted", "Нет активных сеансов."));
    } catch (error) {
      box.replaceChildren(node("p", "notice", translateError(error.message)));
    }
  }

  $("revoke-sessions").addEventListener("click", async () => {
    if (!confirm("Завершить все остальные сеансы?")) return;
    try {
      const data = await api("/sessions/revoke-others", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: "{}"
      });
      toast("Завершено сеансов: " + data.revoked);
      await openSettings();
    } catch (error) {
      toast(translateError(error.message));
    }
  });

  async function boot() {
    if (state.booting) return;
    state.booting = true;
    try {
      if (!state.me) state.me = await api("/me");
      showApp();
      await loadChats();
      connectSocket();
    } catch (error) {
      clearTokens();
      state.me = null;
      showAuth(error.message === "Не удалось связаться с сервером."
        ? "Сервер временно недоступен. Попробуй ещё раз." : "Сессия завершена. Войди снова.");
    } finally {
      state.booting = false;
    }
  }

  if ("serviceWorker" in navigator) {
    window.addEventListener("load", () => {
      navigator.serviceWorker.register("/sw.js", { updateViaCache: "none" })
        .then((registration) => registration.update())
        .catch(() => {});
    }, { once: true });
  }

  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "visible" && state.access && state.chat) connectSocket();
  });

  if (state.access && state.refresh) boot();
  else showAuth();
})();
