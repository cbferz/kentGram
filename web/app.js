(() => {
  "use strict";
  const API = "https://api.kentgram.tendercheck.pro/api/v1";
  const WS = "wss://api.kentgram.tendercheck.pro/api/v1/realtime";
  const $ = (id) => document.getElementById(id);
  const state = { access: sessionStorage.getItem("kg_access") || "", refresh: sessionStorage.getItem("kg_refresh") || "", me: null, chats: [], chat: null, socket: null, authMode: "login", searchTimer: 0, toastTimer: 0 };
  const esc = (s) => String(s ?? "");
  function toast(message) { const el=$("toast"); el.textContent=message; el.classList.add("show"); clearTimeout(state.toastTimer); state.toastTimer=setTimeout(()=>el.classList.remove("show"),2800); }
  function saveTokens(data) { state.access=data.access_token; state.refresh=data.refresh_token; sessionStorage.setItem("kg_access",state.access); sessionStorage.setItem("kg_refresh",state.refresh); }
  function clearTokens() { state.access="";state.refresh="";sessionStorage.removeItem("kg_access");sessionStorage.removeItem("kg_refresh"); }
  async function raw(path, options={}) {
    const headers = new Headers(options.headers || {});
    if (options.auth !== false && state.access) headers.set("Authorization","Bearer "+state.access);
    let response;
    try { response=await fetch(API+path,{...options,headers}); }
    catch { throw new Error("Не удалось связаться с сервером. Проверь подключение."); }
    const data=await response.json().catch(()=>({}));
    if (!response.ok) { const err=new Error(data.error || "request_failed"); err.status=response.status; throw err; }
    return data;
  }
  let refreshing = null;
  async function protectedFetch(url) {
    const send = () => fetch(url,{headers:{Authorization:"Bearer "+state.access}});
    let response = await send();
    if(response.status===401 && state.refresh) {
      if(!refreshing) refreshing=(async()=>{const data=await raw("/auth/refresh",{method:"POST",auth:false,headers:{"Content-Type":"application/json"},body:JSON.stringify({refresh_token:state.refresh})});saveTokens(data);})().finally(()=>refreshing=null);
      await refreshing;
      response=await send();
    }
    if(!response.ok)throw new Error("media_request_failed");
    return response;
  }
  async function api(path, options={}) {
    try { return await raw(path,options); }
    catch (error) {
      if(error.status!==401 || options.auth===false || !state.refresh || path==="/auth/refresh") throw error;
      if(!refreshing) refreshing=(async()=>{const data=await raw("/auth/refresh",{method:"POST",auth:false,headers:{"Content-Type":"application/json"},body:JSON.stringify({refresh_token:state.refresh})});saveTokens(data);return data;})().finally(()=>refreshing=null);
      await refreshing;
      return raw(path,options);
    }
  }
  function showAuth(message="") {
    stopSocket(); $("app").classList.add("hidden");$("auth").classList.remove("hidden");
    $("auth-notice").textContent=message;
  }
  function showApp() { $("auth").classList.add("hidden");$("app").classList.remove("hidden");$("my-name").textContent=state.me.display_name||state.me.username;$("my-handle").textContent="@"+state.me.username;$("my-avatar").textContent=(state.me.display_name||state.me.username||"K").slice(0,1).toUpperCase(); }
  function setAuthMode(mode) {
    state.authMode=mode;const reg=mode==="register";$("auth-title").textContent=reg?"Создать аккаунт":"С возвращением";$("auth-subtitle").textContent=reg?"Зарегистрируй kentGram аккаунт.":"Войди, чтобы продолжить общение.";$("email-field").classList.toggle("hidden",!reg);$("auth-email").required=reg;$("auth-password").autocomplete=reg?"new-password":"current-password";$("auth-submit").textContent=reg?"Зарегистрироваться":"Войти";$("auth-toggle").textContent=reg?"Уже есть аккаунт? Войти":"Создать аккаунт";$("auth-notice").textContent="";
  }
  $("auth-toggle").addEventListener("click",()=>setAuthMode(state.authMode==="login"?"register":"login"));
  $("auth-form").addEventListener("submit",async e=>{
    e.preventDefault();const button=$("auth-submit");button.disabled=true;
    try {
      const username=$("auth-username").value.trim(),password=$("auth-password").value;
      const registering=state.authMode==="register";
      const body={username,password,device_name:"kentGram Web"};
      if(registering)body.email=$("auth-email").value.trim();
      const data=await raw(registering?"/auth/register":"/auth/login",{method:"POST",auth:false,headers:{"Content-Type":"application/json"},body:JSON.stringify(body)});
      saveTokens(data);state.me=data.user||await api("/me");await boot();
    } catch(err) { $("auth-notice").textContent=translateError(err.message); }
    finally { button.disabled=false; }
  });
  function translateError(code) { const map={invalid_credentials:"Неверное имя пользователя или пароль.",username_or_email_taken:"Имя пользователя или почта уже заняты.",invalid_registration_data:"Проверь имя пользователя, почту и пароль (не менее 10 символов).",too_many_login_attempts:"Слишком много попыток. Попробуй позже.",origin_not_allowed:"Источник запроса не разрешён."};return map[code]||"Не удалось выполнить запрос: "+code; }
  function node(tag,cls,text) { const el=document.createElement(tag);if(cls)el.className=cls;if(text!==undefined)el.textContent=text;return el; }
  function avatarText(chat) { return (chat.kind==="direct"?(chat.peer_display_name||chat.peer_username||"?"):(chat.title||chat.username||"?")).slice(0,1).toUpperCase(); }
  function chatName(chat) { return chat.kind==="direct"?(chat.peer_display_name||chat.peer_username||"Диалог"):(chat.title||chat.username||"Сообщество"); }
  async function loadChats() {
    const data=await api("/chats");state.chats=data.chats||[];const list=$("chat-list");list.replaceChildren();
    if(!state.chats.length){list.append(node("p","muted", "Пока нет чатов. Найди человека через поиск выше."));return;}
    for(const chat of state.chats){
      const item=node("button","chat-item"+(state.chat?.id===chat.id?" active":""));item.type="button";
      item.append(node("div","avatar",avatarText(chat)));const preview=node("div","chat-preview");preview.append(node("strong","",chatName(chat)),node("span","",chat.last_message||chat.description||"Начни общение"));item.append(preview);
      if(chat.unread_count>0)item.append(node("span","unread",String(chat.unread_count)));
      item.addEventListener("click",()=>openChat(chat.id));list.append(item);
    }
  }
  function renderMessage(m) {
    const box=node("article","message"+(m.is_mine?" mine":""));
    if(!m.is_mine)box.append(node("span","sender",m.sender_username||"Пользователь"));
    if(m.forwarded_from_username)box.append(node("span","sender","Переслано от @"+m.forwarded_from_username));
    if(m.media_id) {
      const url=API+m.media_url;
      if((m.media_mime||"").startsWith("image/")){const img=node("img");img.alt=m.media_name||"Изображение";img.loading="lazy";box.append(img);protectedFetch(url).then(res=>res.blob()).then(blob=>{if(!img.isConnected)return;const objectURL=URL.createObjectURL(blob);img.src=objectURL;img.addEventListener("load",()=>URL.revokeObjectURL(objectURL),{once:true});}).catch(()=>{img.alt="Не удалось загрузить изображение";});}
      else {const link=node("a","body",m.media_name||"Открыть вложение");link.href="#";link.addEventListener("click",async ev=>{ev.preventDefault();try{const res=await protectedFetch(url);const blob=await res.blob();const objectURL=URL.createObjectURL(blob);const a=document.createElement("a");a.href=objectURL;a.download=m.media_name||"attachment";a.click();setTimeout(()=>URL.revokeObjectURL(objectURL),1000);}catch{toast("Не удалось скачать файл");}});box.append(link);}
    }
    if(m.body)box.append(node("div","body",m.body));
    const date=new Date(m.created_at);box.append(node("time","",Number.isNaN(date.getTime())?"":date.toLocaleString("ru-RU",{hour:"2-digit",minute:"2-digit"})));
    return box;
  }
  async function loadMessages(chatId) {
    const data=await api("/chats/"+chatId+"/messages");if(!state.chat||state.chat.id!==chatId)return;
    const area=$("messages");area.replaceChildren();for(const m of data.messages||[])area.append(renderMessage(m));area.scrollTop=area.scrollHeight;
    const latest=(data.messages||[]).at(-1);if(latest)api("/chats/"+chatId+"/read",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({last_read_message_id:latest.id})}).catch(()=>{});
  }
  async function openChat(id) {
    const chat=state.chats.find(c=>c.id===id);if(!chat)return;state.chat=chat;
    $("welcome").classList.add("hidden");$("chat-header").classList.remove("hidden");$("messages").classList.remove("hidden");$("composer").classList.remove("hidden");$("settings-panel").classList.add("hidden");
    $("chat-title").textContent=chatName(chat);$("chat-subtitle").textContent=chat.kind==="direct"?"@"+chat.peer_username:(chat.member_count+" участников");$("chat-avatar").textContent=avatarText(chat);
    $("app").classList.add("chat-open");await loadMessages(id);await loadChats();connectSocket();
  }
  $("message-input").addEventListener("keydown",e=>{if(e.key==="Enter"&&!e.shiftKey){e.preventDefault();$("composer").requestSubmit();}});
  $("composer").addEventListener("submit",async e=>{
    e.preventDefault();if(!state.chat)return;const input=$("message-input"),text=input.value.trim();if(!text)return;input.value="";
    try{await api("/chats/"+state.chat.id+"/messages",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({text})});await loadMessages(state.chat.id);await loadChats();}
    catch(err){input.value=text;toast(translateError(err.message));}
  });
  $("attachment").addEventListener("change",async e=>{
    const file=e.target.files?.[0];e.target.value="";if(!file||!state.chat)return;
    if(file.size>20*1024*1024){toast("Максимальный размер файла — 20 МБ");return;}
    const form=new FormData();form.append("file",file);const caption=prompt("Подпись к файлу (необязательно):")||"";form.append("caption",caption);
    try{await api("/chats/"+state.chat.id+"/media",{method:"POST",body:form});await loadMessages(state.chat.id);await loadChats();}
    catch(err){toast(translateError(err.message));}
  });
  $("people-search").addEventListener("input",e=>{
    clearTimeout(state.searchTimer);const q=e.target.value.trim();if(q.length<2){$("search-results").classList.add("hidden");return;}
    state.searchTimer=setTimeout(()=>searchPeople(q),350);
  });
  async function searchPeople(q) {
    try{const data=await api("/users/search?q="+encodeURIComponent(q));const box=$("search-results");box.replaceChildren();for(const person of data.users||[]){
      const btn=node("button","person-result");btn.type="button";const av=node("div","avatar",(person.display_name||person.username).slice(0,1).toUpperCase());const info=node("span");info.append(node("strong","",person.display_name||person.username),node("small","","@"+person.username));btn.append(av,info);btn.addEventListener("click",async()=>{try{const result=await api("/chats/direct",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({user_id:person.id})});$("people-search").value="";box.classList.add("hidden");await loadChats();await openChat(result.chat_id);}catch(err){toast(translateError(err.message));}});box.append(btn);
    }if(!box.childElementCount)box.append(node("p","muted","Никого не нашли."));box.classList.remove("hidden");}
    catch(err){toast(translateError(err.message));}
  }
  function stopSocket(){if(state.socket){state.socket.onclose=null;state.socket.close();state.socket=null;}}
  function connectSocket(){
    if(!state.access||!window.WebSocket)return;stopSocket();
    try{const ws=new WebSocket(WS,["kentgram","bearer."+state.access]);state.socket=ws;
      ws.onmessage=event=>{let msg;try{msg=JSON.parse(event.data);}catch{return;}if(msg.chat_id&&state.chat&&msg.chat_id===state.chat.id&&["message.created","message.deleted","chat.read","poll.created","poll.updated","poll.closed"].includes(msg.type)){loadMessages(state.chat.id).catch(()=>{});loadChats().catch(()=>{});}else if(msg.type==="user.presence"||msg.type==="typing"){/* reserved for presence UI */}};
      ws.onopen=()=>{for(const chat of state.chats)ws.send(JSON.stringify({type:"presence.subscribe",chat_id:chat.id}));};
      ws.onclose=()=>{if(state.socket===ws)state.socket=null;};
    }catch{state.socket=null;}
  }
  $("refresh-chats").addEventListener("click",()=>loadChats().catch(e=>toast(translateError(e.message))));
  $("chat-reload").addEventListener("click",()=>state.chat&&loadMessages(state.chat.id).catch(e=>toast(translateError(e.message))));
  $("logout-button").addEventListener("click",async()=>{try{await api("/auth/logout",{method:"POST",headers:{"Content-Type":"application/json"},body:"{}"});}catch{}clearTokens();state.me=null;state.chat=null;showAuth();});
  $("settings-button").addEventListener("click",()=>openSettings());
  $("settings-back").addEventListener("click",()=>{$("settings-panel").classList.add("hidden");$("app").classList.remove("settings-open");if(state.chat)$("app").classList.add("chat-open");});
  async function openSettings(){
    $("app").classList.remove("chat-open");$("app").classList.add("settings-open");$("settings-panel").classList.remove("hidden");
    const box=$("web-sessions");box.replaceChildren(node("p","muted","Загружаем…"));
    try{const data=await api("/sessions");box.replaceChildren();for(const s of data.sessions||[]){const row=node("div","session-row");row.append(node("div","avatar",s.current?"✓":"▣"));const info=node("div");info.append(node("strong","",s.device_name+(s.current?" · это устройство":"")),node("small","",s.ip+" · "+new Date(s.last_used_at).toLocaleString("ru-RU")));row.append(info);if(!s.current){const btn=node("button","quiet-button danger","Завершить");btn.addEventListener("click",async()=>{try{await api("/sessions/"+s.id,{method:"DELETE"});openSettings();}catch(e){toast(translateError(e.message));}});row.append(btn);}box.append(row);}if(!box.childElementCount)box.append(node("p","muted","Нет активных сеансов."));}
    catch(e){box.replaceChildren(node("p","notice",translateError(e.message)));}
  }
  $("revoke-sessions").addEventListener("click",async()=>{if(!confirm("Завершить все остальные сеансы?"))return;try{const data=await api("/sessions/revoke-others",{method:"POST",headers:{"Content-Type":"application/json"},body:"{}"});toast("Завершено сеансов: "+data.revoked);openSettings();}catch(e){toast(translateError(e.message));}});
  async function boot(){
    try{if(!state.me)state.me=await api("/me");showApp();await loadChats();connectSocket();}
    catch(err){clearTokens();showAuth("Сессия завершена. Войди снова.");}
  }
  if(state.access&&state.refresh)boot();else showAuth();
})();