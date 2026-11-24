// TEST-ONLY stand-in for the Go API, used to exercise web/app.js in a real
// browser when the Go server cannot be built (see web/test/README.md).
// It mirrors the JSON contracts and status codes of server/internal/httpapi,
// including DisallowUnknownFields, but it is NOT the backend and proves nothing
// about it. Never deploy it.
import http from "node:http";
import crypto from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const WEB = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const MIME = { ".html": "text/html; charset=utf-8", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".webmanifest": "application/manifest+json" };

export function startMock(port = 0) {
  let uid = 0, cid = 0, mid = 0, nTok = 0;
  const users = new Map(), chats = new Map(), messages = [], reactions = [], pins = [];
  const access = new Map(), refresh = new Map(); // token -> userId
  const conns = new Set(); // {sock, userId}
  const log = { refresh: 0, requests: [] };

  const mkUser = (username, email, password) => { const u = { id: ++uid, username, email, password, display_name: "", bio: "", avatar_url: "", created_at: new Date().toISOString() }; users.set(u.id, u); return u; };
  for (const n of ["alice", "bob", "carol"]) mkUser(n, n + "@x.test", "password-" + n + "-1234");

  const newTokens = (u) => { const a = "acc" + ++nTok, r = "kgr" + crypto.randomBytes(8).toString("hex"); access.set(a, u.id); refresh.set(r, u.id); return { access_token: a, refresh_token: r, token_type: "Bearer", expires_in: 900 }; };
  const members = (c) => c.members;
  const chatFor = (id, u) => { const c = chats.get(id); return c && c.members.includes(u) ? c : null; };
  const visible = (id) => messages.find((m) => m.id === id && !m.deleted_at);

  function view(m, viewer) {
    const reply = m.reply_to_message_id ? visible(m.reply_to_message_id) : null;
    const rs = new Map();
    for (const r of reactions.filter((r) => r.message_id === m.id)) { const e = rs.get(r.emoji) || { emoji: r.emoji, count: 0, mine: false, at: r.at }; e.count++; if (r.user_id === viewer) e.mine = true; rs.set(r.emoji, e); }
    const out = { id: m.id, chat_id: m.chat_id, sender_id: m.sender_id, sender_username: users.get(m.sender_id).username, body: m.body, created_at: m.created_at, is_mine: m.sender_id === viewer, pinned: pins.some((p) => p.chat_id === m.chat_id && p.message_id === m.id), reactions: [...rs.values()].sort((a, b) => a.at - b.at).map(({ emoji, count, mine }) => ({ emoji, count, mine })) };
    if (m.reply_to_message_id) { out.reply_to_message_id = m.reply_to_message_id; if (reply) { out.reply_to_body = reply.body; out.reply_to_sender_username = users.get(reply.sender_id).username; } }
    if (m.edited_at) out.edited_at = m.edited_at;
    if (m.fwd_user) { out.forwarded_from_user_id = m.fwd_user; out.forwarded_from_username = users.get(m.fwd_user).username; }
    return out;
  }
  const neutral = (v) => ({ ...v, is_mine: false, reactions: v.reactions.map((r) => ({ ...r, mine: false })) });
  const lastRead = (c, u) => c.read.get(u) || 0;

  function publish(userIds, ev) {
    const s = JSON.stringify(ev);
    for (const c of conns) if (userIds.includes(c.userId)) c.send(s);
  }
  const toChat = (c, ev, except = 0) => publish(c.members.filter((u) => u !== except), { ...ev, chat_id: c.id });
  const peers = (u) => [...new Set([...chats.values()].filter((c) => c.members.includes(u)).flatMap((c) => c.members).filter((x) => x !== u))];

  /* ---------- http ---------- */
  const send = (res, code, obj, headers = {}) => { res.writeHead(code, { "Content-Type": "application/json", ...headers }); res.end(JSON.stringify(obj)); };
  const readBody = (req, allowed) => new Promise((resolve) => {
    let d = ""; req.on("data", (c) => (d += c));
    req.on("end", () => {
      if (!d) return resolve({});
      try { const j = JSON.parse(d); if (!j || typeof j !== "object" || Object.keys(j).some((k) => !allowed.includes(k))) return resolve(null); resolve(j); } catch { resolve(null); }
    });
  });
  const validEmoji = (s) => typeof s === "string" && [...s].length >= 1 && [...s].length <= 16 && !/\s/.test(s) && /[^\x00-\x7f]/.test(s);
  const validCid = (s) => s === undefined || (typeof s === "string" && /^[A-Za-z0-9_.:-]{0,64}$/.test(s));
  const okText = (s) => typeof s === "string" && [...s.trim()].length >= 1 && [...s.trim()].length <= 4000;

  async function api(req, res, url) {
    const p = url.pathname.replace(/^\/api\/v1/, ""), M = req.method;
    log.requests.push(M + " " + p);
    const bad = () => send(res, 400, { error: "invalid_json" });

    if (p === "/auth/register" && M === "POST") {
      const b = await readBody(req, ["username", "email", "password", "device_name"]); if (!b) return bad();
      if (!/^[A-Za-z0-9_]{3,32}$/.test(b.username || "") || !(b.email || "").includes("@") || (b.password || "").length < 10) return send(res, 400, { error: "invalid_registration_data" });
      if ([...users.values()].some((u) => u.username === b.username || u.email === b.email.toLowerCase())) return send(res, 409, { error: "username_or_email_taken" });
      const u = mkUser(b.username, b.email.toLowerCase(), b.password);
      return send(res, 201, { user: pub(u, true), ...newTokens(u) });
    }
    if (p === "/auth/login" && M === "POST") {
      const b = await readBody(req, ["email", "password", "device_name"]); if (!b) return bad();
      const u = [...users.values()].find((x) => x.email === (b.email || "").toLowerCase());
      if (!u || u.password !== b.password) return send(res, 401, { error: "invalid_credentials" });
      return send(res, 200, { user: pub(u, true), ...newTokens(u) });
    }
    if (p === "/auth/refresh" && M === "POST") {
      log.refresh++;
      const b = await readBody(req, ["refresh_token"]); if (!b) return bad();
      const id = refresh.get(b.refresh_token);
      if (!id) return send(res, 401, { error: "refresh_token_reused" });
      refresh.delete(b.refresh_token); // rotation: reuse is detectable
      return send(res, 200, newTokens(users.get(id)));
    }
    if (p === "/health") return send(res, 200, { status: "ok" });

    const tok = (req.headers.authorization || "").replace(/^Bearer /, "");
    const me = access.get(tok);
    if (!me) return send(res, 401, { error: "unauthorized" });

    if (p === "/me" && M === "GET") return send(res, 200, pub(users.get(me), true));
    if (p === "/me/profile" && M === "PATCH") {
      const b = await readBody(req, ["display_name", "bio", "avatar_url"]); if (!b) return bad();
      if ((b.display_name || "").length > 64 || (b.bio || "").length > 160) return send(res, 400, { error: "profile_field_too_long" });
      if (b.avatar_url && !/^https:\/\//.test(b.avatar_url)) return send(res, 400, { error: "avatar_url_must_be_https" });
      Object.assign(users.get(me), { display_name: b.display_name || "", bio: b.bio || "", avatar_url: b.avatar_url || "" });
      return send(res, 200, pub(users.get(me), true));
    }
    if (p === "/users/search" && M === "GET") {
      const q = (url.searchParams.get("q") || "").trim().toLowerCase();
      if (q.length < 2 || q.length > 64) return send(res, 400, { error: "query_must_be_2_to_64_characters" });
      return send(res, 200, { users: [...users.values()].filter((u) => u.id !== me && (u.username.includes(q) || u.display_name.toLowerCase().includes(q))).map((u) => pub(u)) });
    }
    let m;
    if ((m = p.match(/^\/users\/([^/]+)$/)) && M === "GET") { const u = [...users.values()].find((x) => x.username === m[1]); return u ? send(res, 200, pub(u)) : send(res, 404, { error: "user_not_found" }); }
    if ((m = p.match(/^\/users\/(\d+)\/follow$/))) {
      const t = users.get(+m[1]); if (!t) return send(res, 404, { error: "user_not_found" });
      if (t.id === me) return send(res, 400, { error: "cannot_follow_self" });
      follows.add(me + ">" + t.id, M === "POST"); return send(res, 200, { success: true, following: M === "POST" });
    }
    if (p === "/contacts") return send(res, 200, { contacts: [...users.values()].filter((u) => follows.has(me + ">" + u.id)).map((u) => pub(u)) });
    if (p === "/sessions") return send(res, 200, { sessions: [{ id: 1, device_name: "Web", user_agent: "", ip: "127.0.0.1", created_at: new Date().toISOString(), last_used_at: new Date().toISOString(), expires_at: new Date().toISOString(), current: true }] });
    if (p === "/auth/logout" && M === "POST") { access.delete(tok); for (const c of conns) if (c.userId === me) c.close(); return send(res, 200, { success: true }); }
    if (p === "/presence") return send(res, 200, { online: peers(me).filter((u) => [...conns].some((c) => c.userId === u)) });

    if (p === "/chats" && M === "GET") {
      const list = [...chats.values()].filter((c) => c.members.includes(me)).map((c) => {
        const peer = users.get(c.members.find((x) => x !== me)), msgs = messages.filter((x) => x.chat_id === c.id && !x.deleted_at), last = msgs[msgs.length - 1];
        return { id: c.id, kind: "direct", peer_id: peer.id, peer_username: peer.username, peer_display_name: peer.display_name, peer_avatar_url: peer.avatar_url, last_message: last ? last.body : "", ...(last ? { last_message_at: last.created_at } : {}), unread_count: msgs.filter((x) => x.sender_id !== me && x.id > lastRead(c, me)).length, created_at: c.created_at, _t: last ? last.created_at : c.created_at };
      }).sort((a, b) => (a._t < b._t ? 1 : -1)).map(({ _t, ...r }) => r);
      return send(res, 200, { chats: list });
    }
    if (p === "/chats/direct" && M === "POST") {
      const b = await readBody(req, ["user_id"]); if (!b) return bad();
      if (!users.has(b.user_id) || b.user_id === me) return send(res, 404, { error: "user_not_found" });
      let c = [...chats.values()].find((x) => x.members.includes(me) && x.members.includes(b.user_id));
      if (!c) { c = { id: ++cid, members: [me, b.user_id], read: new Map(), created_at: new Date().toISOString() }; chats.set(c.id, c); }
      return send(res, 200, { chat_id: c.id });
    }

    if ((m = p.match(/^\/chats\/(\d+)\/(messages|pins|read|typing)(?:\/(\d+))?(?:\/(reactions|forward))?$/))) {
      const chatId = +m[1], kind = m[2], msgId = m[3] ? +m[3] : 0, sub = m[4];
      const c = chatFor(chatId, me);
      if (kind === "typing" && M === "POST") { if (!c) return send(res, 404, { error: "chat_not_found" }); toChat(c, { type: "typing", data: { user_id: me } }, me); return send(res, 200, { ok: true }); }
      if (kind === "read" && M === "POST") {
        const b = await readBody(req, ["message_id"]); if (!b) return bad(); if (!c) return send(res, 404, { error: "chat_not_found" });
        const max = Math.max(0, ...messages.filter((x) => x.chat_id === c.id).map((x) => x.id));
        const v = Math.max(lastRead(c, me), Math.min(b.message_id, max)); c.read.set(me, v);
        toChat(c, { type: "chat.read", data: { user_id: me, last_read_message_id: v } });
        return send(res, 200, { last_read_message_id: v });
      }
      if (kind === "pins" && !msgId && M === "GET") {
        if (!c) return send(res, 404, { error: "chat_not_found" });
        return send(res, 200, { messages: pins.filter((x) => x.chat_id === c.id).sort((a, b) => b.at - a.at).map((x) => visible(x.message_id)).filter(Boolean).map((x) => view(x, me)) });
      }
      if (kind === "pins" && msgId) {
        const ok = c && visible(msgId)?.chat_id === chatId;
        if (M === "PUT") { if (!ok) return send(res, 404, { error: "message_not_found" }); if (!pins.some((x) => x.message_id === msgId)) pins.push({ chat_id: chatId, message_id: msgId, at: ++mid + Date.now() }); toChat(c, { type: "message.pinned", data: { message_id: msgId, actor_id: me } }); return send(res, 200, { pinned: true }); }
        if (M === "DELETE") { if (!c) return send(res, 404, { error: "chat_not_found" }); const i = pins.findIndex((x) => x.message_id === msgId); if (i >= 0) pins.splice(i, 1); toChat(c, { type: "message.unpinned", data: { message_id: msgId, actor_id: me } }); return send(res, 200, { pinned: false }); }
      }
      if (kind === "messages" && !msgId) {
        if (M === "GET") {
          if (!c) return send(res, 404, { error: "chat_not_found" });
          const before = +url.searchParams.get("before_id") || 0;
          const page = messages.filter((x) => x.chat_id === c.id && !x.deleted_at && (!before || x.id < before)).slice(-50);
          const max = Math.max(0, ...messages.filter((x) => x.chat_id === c.id).map((x) => x.id)); c.read.set(me, Math.max(lastRead(c, me), max));
          toChat(c, { type: "chat.read", data: { user_id: me, last_read_message_id: lastRead(c, me) } }, me);
          const peerRead = Math.max(0, ...c.members.filter((x) => x !== me).map((x) => lastRead(c, x)));
          return send(res, 200, { messages: page.map((x) => view(x, me)), peer_last_read_message_id: peerRead });
        }
        if (M === "POST") {
          const b = await readBody(req, ["text", "reply_to_message_id", "client_id"]); if (!b || !validCid(b.client_id)) return bad();
          if (!okText(b.text)) return send(res, 400, { error: "message_must_be_1_to_4000_characters" });
          if (!c) return send(res, 404, { error: "chat_or_reply_not_found" });
          if (b.client_id) { const dup = messages.find((x) => x.sender_id === me && x.client_id === b.client_id); if (dup) return dup.chat_id === chatId ? send(res, 200, view(dup, me)) : send(res, 409, { error: "client_id_conflict" }); }
          if (b.reply_to_message_id && visible(b.reply_to_message_id)?.chat_id !== chatId) return send(res, 404, { error: "chat_or_reply_not_found" });
          const nm = { id: ++mid, chat_id: chatId, sender_id: me, body: b.text.trim(), created_at: new Date().toISOString(), client_id: b.client_id, reply_to_message_id: b.reply_to_message_id || 0 };
          if (!b.reply_to_message_id) delete nm.reply_to_message_id;
          if (opts.delaySend) await new Promise((r) => setTimeout(r, opts.delaySend));
          messages.push(nm); toChat(c, { type: "message.new", data: neutral(view(nm, me)) });
          return send(res, 201, view(nm, me));
        }
      }
      if (kind === "messages" && msgId && !sub) {
        const mm = messages.find((x) => x.id === msgId && x.chat_id === chatId && !x.deleted_at);
        if (M === "PATCH") {
          const b = await readBody(req, ["text"]); if (!b) return bad(); if (!okText(b.text)) return send(res, 400, { error: "message_must_be_1_to_4000_characters" });
          if (!c || !mm || mm.sender_id !== me) return send(res, 404, { error: "message_not_found" });
          mm.body = b.text.trim(); mm.edited_at = new Date().toISOString(); toChat(c, { type: "message.edited", data: neutral(view(mm, me)) });
          return send(res, 200, view(mm, me));
        }
        if (M === "DELETE") {
          if (!c || !mm || mm.sender_id !== me) return send(res, 404, { error: "message_not_found" });
          mm.deleted_at = new Date().toISOString(); const i = pins.findIndex((x) => x.message_id === msgId); if (i >= 0) pins.splice(i, 1);
          toChat(c, { type: "message.deleted", data: { message_id: msgId } }); return send(res, 200, { deleted: true });
        }
      }
      if (kind === "messages" && msgId && sub === "reactions") {
        const mm = c && messages.find((x) => x.id === msgId && x.chat_id === chatId && !x.deleted_at);
        let emoji, b;
        if (M === "POST") { b = await readBody(req, ["emoji"]); if (!b) return bad(); emoji = (b.emoji || "").trim(); } else emoji = (url.searchParams.get("emoji") || "").trim();
        if (!validEmoji(emoji)) return send(res, 400, { error: "invalid_emoji" });
        if (!mm) return send(res, 404, { error: "message_not_found" });
        if (M === "POST") {
          if (!reactions.some((r) => r.message_id === msgId && r.user_id === me && r.emoji === emoji)) reactions.push({ message_id: msgId, user_id: me, emoji, at: ++mid });
          const mine = reactions.filter((r) => r.message_id === msgId && r.user_id === me).sort((a, b) => a.at - b.at); while (mine.length > 3) { reactions.splice(reactions.indexOf(mine.shift()), 1); }
          if (new Set(reactions.filter((r) => r.message_id === msgId).map((r) => r.emoji)).size > 10) { reactions.pop(); return send(res, 409, { error: "reaction_limit_reached" }); }
        } else { const i = reactions.findIndex((r) => r.message_id === msgId && r.user_id === me && r.emoji === emoji); if (i >= 0) reactions.splice(i, 1); }
        const counts = [...new Map(reactions.filter((r) => r.message_id === msgId).sort((a, b) => a.at - b.at).map((r) => [r.emoji, 0])).keys()].map((e) => ({ emoji: e, count: reactions.filter((r) => r.message_id === msgId && r.emoji === e).length }));
        const actor = reactions.filter((r) => r.message_id === msgId && r.user_id === me).map((r) => r.emoji);
        toChat(c, { type: "reaction.updated", data: { message_id: msgId, actor_id: me, emoji, added: M === "POST", counts, actor_emojis: actor } });
        return send(res, 200, { message_id: msgId, counts, actor_emojis: actor });
      }
      if (kind === "messages" && msgId && sub === "forward" && M === "POST") {
        const b = await readBody(req, ["to_chat_id", "client_id"]); if (!b || !validCid(b.client_id)) return bad();
        const dst = chatFor(b.to_chat_id, me), src = c && messages.find((x) => x.id === msgId && x.chat_id === chatId && !x.deleted_at);
        if (!dst || !src) return send(res, 404, { error: "message_or_chat_not_found" });
        if (b.client_id) { const dup = messages.find((x) => x.sender_id === me && x.client_id === b.client_id); if (dup) return send(res, 200, view(dup, me)); }
        const nm = { id: ++mid, chat_id: dst.id, sender_id: me, body: src.body, created_at: new Date().toISOString(), client_id: b.client_id, fwd_user: src.fwd_user || src.sender_id };
        messages.push(nm); toChat(dst, { type: "message.new", data: neutral(view(nm, me)) });
        return send(res, 201, view(nm, me));
      }
    }
    return send(res, 404, { error: "endpoint_not_found" });
  }
  const follows = Object.assign(new Set(), { add(k, on) { on ? Set.prototype.add.call(this, k) : this.delete(k); return this; } });
  const pub = (u, full) => ({ id: u.id, username: u.username, ...(full ? { email: u.email } : {}), display_name: u.display_name, bio: u.bio, avatar_url: u.avatar_url, created_at: u.created_at, followers: [...follows].filter((k) => k.endsWith(">" + u.id)).length, following: [...follows].filter((k) => k.startsWith(u.id + ">")).length, is_following: false });
  const opts = { delaySend: 0 };

  /* ---------- server + WebSocket ---------- */
  const server = http.createServer(async (req, res) => {
    const url = new URL(req.url, "http://x");
    if (url.pathname === "/__test/expire-access") { access.clear(); return send(res, 200, { ok: true }); }
    if (url.pathname === "/__test/drop-ws") { for (const c of conns) c.sock.destroy(); return send(res, 200, { ok: true }); }
    if (url.pathname === "/__test/state") return send(res, 200, { refresh: log.refresh, requests: log.requests, messages: messages.length });
    if (url.pathname === "/__test/delay") { opts.delaySend = +url.searchParams.get("ms") || 0; return send(res, 200, { ok: true }); }
    if (url.pathname === "/health" || url.pathname.startsWith("/api/v1/")) return api(req, res, url).catch((e) => send(res, 500, { error: "mock_failure: " + e.message }));
    let f = path.join(WEB, url.pathname === "/" ? "index.html" : url.pathname);
    if (!f.startsWith(WEB) || f.includes(path.sep + "test" + path.sep) || !fs.existsSync(f) || fs.statSync(f).isDirectory()) { res.writeHead(404); return res.end("not found"); }
    res.writeHead(200, { "Content-Type": MIME[path.extname(f)] || "application/octet-stream", "Cache-Control": "no-cache" }); fs.createReadStream(f).pipe(res);
  });

  server.on("upgrade", (req, sock) => {
    if (new URL(req.url, "http://x").pathname !== "/api/v1/ws") return sock.destroy();
    const key = req.headers["sec-websocket-key"];
    sock.write("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " + crypto.createHash("sha1").update(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11").digest("base64") + "\r\n\r\n");
    const conn = { sock, userId: 0, send: (s) => { const b = Buffer.from(s), h = b.length < 126 ? Buffer.from([0x81, b.length]) : Buffer.from([0x81, 126, b.length >> 8, b.length & 255]); sock.write(Buffer.concat([h, b])); }, close: () => sock.end(Buffer.from([0x88, 0])) };
    let buf = Buffer.alloc(0), authed = false;
    const authTimer = setTimeout(() => sock.destroy(), 5000);
    sock.on("data", (d) => {
      buf = Buffer.concat([buf, d]);
      while (buf.length >= 2) {
        let len = buf[1] & 127, off = 2; if (len === 126) { if (buf.length < 4) return; len = buf.readUInt16BE(2); off = 4; }
        if (buf.length < off + 4 + len) return;
        const mask = buf.subarray(off, off + 4), payload = Buffer.from(buf.subarray(off + 4, off + 4 + len)).map((v, i) => v ^ mask[i & 3]); const op = buf[0] & 15; buf = buf.subarray(off + 4 + len);
        if (op === 8) return sock.end();
        if (op === 1 && !authed) {
          let j; try { j = JSON.parse(payload.toString()); } catch { return sock.destroy(); }
          const id = access.get(j.token); if (j.type !== "auth" || !id) return sock.destroy();
          authed = true; clearTimeout(authTimer); conn.userId = id;
          const first = ![...conns].some((c) => c.userId === id); conns.add(conn);
          conn.send(JSON.stringify({ type: "ready", data: { user_id: id } }));
          if (first) publish(peers(id), { type: "presence", data: { user_id: id, online: true } });
        }
      }
    });
    sock.on("close", () => { clearTimeout(authTimer); if (!conns.delete(conn) || !conn.userId) return; if (![...conns].some((c) => c.userId === conn.userId)) publish(peers(conn.userId), { type: "presence", data: { user_id: conn.userId, online: false } }); });
    sock.on("error", () => {});
  });

  return new Promise((resolve) => server.listen(port, "127.0.0.1", () => resolve({ port: server.address().port, url: "http://127.0.0.1:" + server.address().port, close: () => { for (const c of conns) c.sock.destroy(); server.close(); }, seed: { chats, users } })));
}

if (process.argv[1] === fileURLToPath(import.meta.url)) startMock(+process.env.PORT || 8099).then((s) => console.log("mock API + web on " + s.url));
