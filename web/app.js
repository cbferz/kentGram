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
      const endpoint = (options.method || "GET").toUpperCase() + " " + path;
      const code = data.error || "request_failed";
      const error = new Error((code === "endpoint_not_found" || code === "method_not_allowed")
        ? code + " · " + endpoint : code);
      error.status = response.status;
      error.endpoint = endpoint;
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
    const avatar = $("my-avatar");
    const avatarFallback = (state.me.display_name || state.me.username || "K").slice(0, 1).toUpperCase();
    setAvatar(avatar, state.me.avatar_url, avatarFallback);
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

  function setAvatar(element, url, fallback) {
    if (!element) return;
    if (url) {
      const image = document.createElement("img");
      image.src = url;
      image.alt = "";
      image.loading = "lazy";
      image.onerror = () => { element.textContent = fallback || "K"; };
      element.replaceChildren(image);
    } else {
      element.textContent = fallback || "K";
    }
  }

  function avatarText(chat) {
    return chatName(chat).slice(0, 1).toUpperCase() || "K";
  }

  async function loadChats() {
    const data = await api("/chats");
    state.chats = Array.isArray(data.chats) ? data.chats : [];
    if (state.chat) {
      const refreshedChat = state.chats.find(chat => chat.id === state.chat.id);
      if (refreshedChat) state.chat = refreshedChat;
    }
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
      const avatar = node("div", "avatar");
      setAvatar(avatar, chat.kind === "direct" ? chat.peer_avatar_url : "", avatarText(chat));
      item.append(avatar);
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
    await loadPolls(chatId);
  }

  async function loadPolls(chatId) {
    const data = await api("/chats/" + encodeURIComponent(chatId) + "/polls");
    if (!state.chat || state.chat.id !== chatId) return;
    const area = $("messages");
    for (const poll of (Array.isArray(data.polls) ? data.polls : [])) {
      const card = node("section", "poll-card");
      const heading = node("div", "poll-heading");
      heading.append(node("strong", "", poll.question));
      heading.append(node("span", "poll-meta",
        (poll.closed ? "Завершён · " : "") + (poll.total_votes || 0) + " голосов" +
        (poll.anonymous ? " · анонимно" : "")));
      card.append(heading);

      const choices = node("div", "poll-options");
      const controls = [];
      for (const option of (poll.options || [])) {
        const label = node("label", "poll-option");
        const control = document.createElement("input");
        control.type = poll.multiple_choice ? "checkbox" : "radio";
        control.name = "poll-" + poll.id;
        control.value = String(option.id);
        control.checked = !!option.selected;
        control.disabled = !!poll.closed;
        const copy = node("span", "poll-option-copy");
        copy.append(node("span", "poll-option-text", option.text));
        copy.append(node("span", "poll-option-votes", String(option.votes || 0)));
        label.append(control, copy);
        choices.append(label);
        controls.push(control);
      }
      card.append(choices);

      const footer = node("div", "poll-footer");
      const vote = node("button", "secondary poll-vote", poll.closed ? "Опрос завершён" : "Голосовать");
      vote.type = "button";
      vote.disabled = !!poll.closed;
      vote.addEventListener("click", async () => {
        const optionIds = controls.filter(control => control.checked).map(control => Number(control.value));
        if (!optionIds.length) { toast("Выбери вариант ответа"); return; }
        vote.disabled = true;
        try {
          await api("/chats/" + encodeURIComponent(chatId) + "/polls/" +
            encodeURIComponent(poll.id) + "/vote", {
              method: "POST", headers: { "Content-Type": "application/json" },
              body: JSON.stringify({ option_ids: optionIds })
            });
          await loadMessages(chatId);
        } catch (error) {
          vote.disabled = false;
          toast(translateError(error.message));
        }
      });
      footer.append(vote);

      if (!poll.closed && (Number(poll.creator_id) === Number(state.me?.id) ||
          ["owner", "admin"].includes(state.chat.role))) {
        const close = node("button", "message-action danger", "Завершить");
        close.type = "button";
        close.addEventListener("click", async () => {
          try {
            await api("/chats/" + encodeURIComponent(chatId) + "/polls/" +
              encodeURIComponent(poll.id) + "/close", { method: "POST" });
            await loadMessages(chatId);
          } catch (error) { toast(translateError(error.message)); }
        });
        footer.append(close);
      }
      card.append(footer);
      area.append(card);
    }
  }

  async function createCommunity() {
    const kind = confirm("Создать канал? Нажми «Отмена» для создания группы.") ? "channel" : "group";
    const title = prompt(kind === "channel" ? "Название канала:" : "Название группы:");
    if (!title || !title.trim()) return;
    const description = prompt("Описание (необязательно):") || "";
    const isPublic = confirm("Сделать сообщество публичным?");
    let username = "";
    if (isPublic) {
      username = (prompt("Публичный адрес без @:") || "").trim().replace(/^@/, "");
      if (!username) return;
    }
    try {
      const created = await api("/chats/community", {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ kind, title: title.trim(), description, is_public: isPublic, username })
      });
      await loadChats();
      if (created.id) await openChat(Number(created.id));
      toast(kind === "channel" ? "Канал создан" : "Группа создана");
    } catch (error) { toast(translateError(error.message)); }
  }

  async function joinCommunity() {
    const username = (prompt("Публичный адрес сообщества (без @):") || "").trim().replace(/^@/, "");
    if (!username) return;
    try {
      const result = await api("/chats/join", {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username })
      });
      await loadChats();
      if (result.chat_id) await openChat(Number(result.chat_id));
    } catch (error) { toast(translateError(error.message)); }
  }

  async function manageMembers() {
    if (!state.chat || state.chat.kind === "direct") return;
    const chatId = state.chat.id;
    try {
      const data = await api("/chats/" + encodeURIComponent(chatId) + "/members");
      const members = Array.isArray(data.members) ? data.members : [];
      const listing = members.map(member =>
        "@" + member.username + " · " + (member.display_name || member.username) + " · " + member.role
      ).join("\n");
      const action = (prompt(
        "Участники:\n" + (listing || "Пока никого") +
        "\n\nДействие: add / remove / role / transfer / leave"
      ) || "").trim().toLowerCase();
      if (!action) return;

      if (action === "leave") {
        if (!confirm("Выйти из этого сообщества?")) return;
        await api("/chats/" + encodeURIComponent(chatId) + "/leave", { method: "POST" });
        closeChatView();
        await loadChats();
        return;
      }

      const username = (prompt("Username участника:") || "").trim().replace(/^@/, "");
      if (!username) return;
      const member = members.find(item => String(item.username).toLowerCase() === username.toLowerCase());
      if (action === "add") {
        const profile = await api("/users/" + encodeURIComponent(username));
        await api("/chats/" + encodeURIComponent(chatId) + "/members", {
          method: "POST", headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ user_id: profile.id })
        });
      } else {
        if (!member) { toast("Участник не найден в этом чате"); return; }
        if (action === "remove") {
          if (!confirm("Удалить @" + username + " из сообщества?")) return;
          await api("/chats/" + encodeURIComponent(chatId) + "/members/" +
            encodeURIComponent(member.user_id), { method: "DELETE" });
        } else if (action === "role") {
          const role = (prompt("Новая роль: admin или member") || "").trim().toLowerCase();
          if (!["admin", "member"].includes(role)) return;
          await api("/chats/" + encodeURIComponent(chatId) + "/members/" +
            encodeURIComponent(member.user_id) + "/role", {
              method: "PATCH", headers: { "Content-Type": "application/json" },
              body: JSON.stringify({ role })
            });
        } else if (action === "transfer") {
          if (!confirm("Передать владение @" + username + "?")) return;
          await api("/chats/" + encodeURIComponent(chatId) + "/transfer-ownership", {
            method: "POST", headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ user_id: member.user_id })
          });
        } else {
          toast("Неизвестное действие");
          return;
        }
      }
      await loadChats();
      if (state.chat?.id === chatId) await loadMessages(chatId);
      toast("Изменения участников сохранены");
    } catch (error) { toast(translateError(error.message)); }
  }

  async function createPoll() {
    if (!state.chat) return;
    const question = prompt("Вопрос опроса:");
    if (!question || !question.trim()) return;
    const rawOptions = prompt("Варианты ответа через символ | (минимум два):");
    if (!rawOptions) return;
    const options = rawOptions.split("|").map(value => value.trim()).filter(Boolean);
    if (options.length < 2 || options.length > 10) {
      toast("Нужно от 2 до 10 вариантов");
      return;
    }
    try {
      await api("/chats/" + encodeURIComponent(state.chat.id) + "/polls", {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          question: question.trim(), options,
          multiple_choice: confirm("Разрешить выбирать несколько вариантов?"),
          anonymous: confirm("Сделать опрос анонимным?")
        })
      });
      await loadMessages(state.chat.id);
    } catch (error) { toast(translateError(error.message)); }
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
    setAvatar($("chat-avatar"), chat.kind === "direct" ? chat.peer_avatar_url : "", avatarText(chat));
    $("manage-members-button").hidden = chat.kind === "direct";
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

  $("create-community-button").addEventListener("click", createCommunity);
  $("join-community-button").addEventListener("click", joinCommunity);
  $("manage-members-button").addEventListener("click", manageMembers);
  $("create-poll-button").addEventListener("click", createPoll);

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
        const avatar = node("div", "avatar");
        setAvatar(avatar, person.avatar_url, (person.display_name || person.username || "?").slice(0, 1).toUpperCase());
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
    $("profile-display-name").value = state.me?.display_name || "";
    $("profile-bio").value = state.me?.bio || "";
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

  $("profile-form").addEventListener("submit", async (event) => {
    event.preventDefault();
    try {
      state.me = await api("/me/profile", {
        method: "PATCH", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          display_name: $("profile-display-name").value.trim(),
          bio: $("profile-bio").value.trim(),
          avatar_url: state.me?.avatar_url || ""
        })
      });
      showApp();
      toast("Профиль сохранён");
    } catch (error) { toast(translateError(error.message)); }
  });

  $("upload-avatar-button").addEventListener("click", async () => {
    const file = $("profile-avatar-file").files?.[0];
    if (!file) { toast("Выбери изображение"); return; }
    if (file.size > 5 * 1024 * 1024) { toast("Аватар должен быть меньше 5 МБ"); return; }
    const form = new FormData();
    form.append("avatar", file);
    try {
      state.me = await api("/me/avatar", { method: "POST", body: form });
      $("profile-avatar-file").value = "";
      showApp();
      toast("Аватар обновлён");
    } catch (error) { toast(translateError(error.message)); }
  });

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
