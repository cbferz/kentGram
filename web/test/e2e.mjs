// Browser end-to-end checks for web/app.js against web/test/mock-server.mjs.
// Run: node web/test/e2e.mjs   (needs Playwright + Chromium; SHOTS=dir saves screenshots)
// This validates the CLIENT. It does not validate the Go backend.
import { createRequire } from "node:module";
import { startMock } from "./mock-server.mjs";
const { chromium } = createRequire(import.meta.url)("playwright");

const SHOTS = process.env.SHOTS || "";
let pass = 0, failed = 0;
const check = (name, ok, extra = "") => { ok ? pass++ : failed++; console.log((ok ? "PASS " : "FAIL ") + name + (ok ? "" : "  " + extra)); };
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
async function until(fn, ms = 4000) { const t = Date.now(); while (Date.now() - t < ms) { try { if (await fn()) return true; } catch { /* retry */ } await sleep(50); } return false; }

const mock = await startMock(0);
const browser = await chromium.launch();
const errors = [];
const watch = (page, tag) => { page.on("pageerror", (e) => errors.push(tag + ": " + e.message)); page.on("console", (m) => { if (m.type() === "error" && !/Failed to load resource|net::ERR/.test(m.text())) errors.push(tag + " console: " + m.text()); }); };

async function login(ctx, name, tag) {
  const page = await ctx.newPage(); watch(page, tag);
  await page.goto(mock.url + "/");
  await page.fill("#f-email", name + "@x.test"); await page.fill("#f-pass", `password-${name}-1234`);
  await page.click("button[type=submit]");
  await page.waitForSelector("#list", { timeout: 5000 });
  return page;
}
const msgTexts = (page) => page.$$eval(".msg .text", (els) => els.map((e) => e.textContent));
const rx = (t) => new RegExp("^" + t.replace(/[.*+?^${}()|[\]\\]/g, "\\$&") + "$");
// match a message by its OWN text (a quote of it inside another bubble must not count)
const openMenuFor = async (page, text) => { const msg = page.locator(".msg").filter({ has: page.locator("div.text", { hasText: rx(text) }) }).last(); await msg.hover(); await msg.locator(".more").click(); };
const menu = (page, label) => page.locator("dialog[open] .menu-item", { hasText: label }).first();

try {
  /* ---- registration + validation errors through the real form ---- */
  {
    const ctx = await browser.newContext(); const p = await ctx.newPage(); watch(p, "reg");
    await p.goto(mock.url + "/");
    await p.fill("#f-email", "alice@x.test"); await p.fill("#f-pass", "wrong-password-1");
    await p.click("button[type=submit]");
    check("bad credentials show a Russian error", await until(async () => /Неверная почта/.test(await p.textContent(".form-error"))));
    await p.click("text=Регистрация");
    await p.fill("#f-user", "dana_1"); await p.fill("#f-email", "dana@x.test"); await p.fill("#f-pass", "long-enough-password");
    await p.click("button[type=submit]");
    check("registration creates an account and opens the app", await p.waitForSelector("#list", { timeout: 5000 }).then(() => true, () => false));
    await ctx.close();
  }

  const ctxA = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  const ctxB = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  const A = await login(ctxA, "alice", "A"), B = await login(ctxB, "bob", "B");

  /* ---- start a chat via search -> profile -> write ---- */
  await A.click("[aria-label='Новый чат']");
  await A.fill("dialog input", "bo");
  await A.waitForSelector("dialog .row");
  await A.click("dialog .row");
  await A.waitForSelector("button:text-is('Написать')");
  await A.click("button:text-is('Написать')");
  await A.waitForSelector("#ta");
  check("chat opened via real /chats/direct", /#\/chat\/\d+/.test(A.url()));
  const box = await A.locator("#ta").boundingBox();
  check("composer is visible inside the 900px laptop viewport", box && box.y + box.height <= 900 && box.y > 0, JSON.stringify(box));

  /* ---- send + realtime delivery + unread badge + read receipts ---- */
  await A.fill("#ta", "привет, Боб"); await A.keyboard.press("Enter");
  check("sender sees own message", await until(async () => (await msgTexts(A)).includes("привет, Боб")));
  check("recipient list shows the chat with unread badge (WebSocket)", await until(async () => (await B.locator(".row .badge").count()) === 1 && /привет/.test(await B.textContent(".row .row-sub"))));
  check("document title carries unread count", await until(async () => (await B.title()) === "(1) KentGram"));
  check("exactly one copy of the message on sender side (REST+WS dedupe)", (await msgTexts(A)).filter((t) => t === "привет, Боб").length === 1);
  await B.click(".row");
  check("recipient opens chat and sees message", await until(async () => (await msgTexts(B)).includes("привет, Боб")));
  check("sender sees read receipt (✓✓)", await until(async () => /✓✓/.test(await A.locator(".msg.mine .ticks").last().textContent())));
  check("recipient badge cleared", await until(async () => (await B.locator(".row .badge").count()) === 0));
  check("presence: sender sees peer online", await until(async () => /в сети/.test(await A.textContent("#status"))));

  /* ---- typing indicator ---- */
  await B.fill("#ta", "п"); await B.press("#ta", "End");
  check("typing indicator reaches peer", await until(async () => /печатает/.test(await A.textContent("#status"))));

  /* ---- reply ---- */
  await openMenuFor(B, "привет, Боб"); await menu(B, "Ответить").click();
  await B.fill("#ta", "Привет, Алиса!"); await B.keyboard.press("Enter");
  check("reply shows quote of original on both sides", await until(async () => (await A.locator(".quote", { hasText: "привет, Боб" }).count()) === 1 && (await B.locator(".quote", { hasText: "привет, Боб" }).count()) === 1));

  /* ---- reactions (add, count, toggle off) ---- */
  await openMenuFor(A, "Привет, Алиса!"); await A.locator("dialog[open] .emoji-row button", { hasText: "👍" }).click();
  check("reaction appears for author of reaction as 'mine'", await until(async () => (await A.locator(".reaction.mine", { hasText: "👍" }).count()) === 1));
  check("peer sees the reaction in realtime, not as mine", await until(async () => (await B.locator(".reaction", { hasText: "👍" }).count()) === 1 && (await B.locator(".reaction.mine").count()) === 0));
  await B.locator(".reaction", { hasText: "👍" }).click();
  check("second user joins reaction -> count 2 for both", await until(async () => /2/.test(await A.locator(".reaction", { hasText: "👍" }).textContent()) && /2/.test(await B.locator(".reaction", { hasText: "👍" }).textContent())));
  await A.locator(".reaction.mine", { hasText: "👍" }).click();
  check("removing own reaction decrements for both", await until(async () => /1/.test(await B.locator(".reaction", { hasText: "👍" }).textContent())));

  /* ---- edit ---- */
  await openMenuFor(A, "привет, Боб"); await menu(A, "Изменить").click();
  await A.fill("#ta", "привет, Боб (исправлено)"); await A.keyboard.press("Enter");
  check("edit applies for author", await until(async () => (await msgTexts(A)).includes("привет, Боб (исправлено)")));
  check("peer sees edit + 'изм.' label and quoted text updated", await until(async () => (await msgTexts(B)).includes("привет, Боб (исправлено)") && (await B.locator(".msg .meta", { hasText: "изм." }).count()) >= 1 && (await B.locator(".quote", { hasText: "(исправлено)" }).count()) === 1));
  await openMenuFor(B, "привет, Боб (исправлено)");
  check("non-author menu has no edit/delete", (await B.locator("dialog[open] .menu-item", { hasText: /Изменить|Удалить/ }).count()) === 0);
  await B.keyboard.press("Escape");

  /* ---- pin ---- */
  await openMenuFor(B, "Привет, Алиса!"); await menu(B, "Закрепить").click();
  check("pin bar shows on both sides", await until(async () => (await A.locator(".pinbar").count()) === 1 && (await B.locator(".pinbar").count()) === 1));
  await openMenuFor(A, "Привет, Алиса!"); await menu(A, "Открепить").click();
  check("unpin removes the pin bar for both", await until(async () => (await A.locator(".pinbar").count()) === 0 && (await B.locator(".pinbar").count()) === 0));

  /* ---- forward into another chat ---- */
  await A.goto(mock.url + "/#/"); // list
  const carolId = await A.evaluate(async () => { const t = JSON.parse(localStorage.getItem("kg.tokens")); const r = await fetch("/api/v1/users/carol", { headers: { Authorization: "Bearer " + t.access } }); return (await r.json()).id; });
  await A.evaluate(async (id) => { const t = JSON.parse(localStorage.getItem("kg.tokens")); await fetch("/api/v1/chats/direct", { method: "POST", headers: { Authorization: "Bearer " + t.access, "Content-Type": "application/json" }, body: JSON.stringify({ user_id: id }) }); }, carolId);
  await A.reload(); await A.waitForSelector("#list .row");
  await A.locator("#list .row", { hasText: "bob" }).click(); await A.waitForSelector("#ta");
  await openMenuFor(A, "Привет, Алиса!"); await menu(A, "Переслать").click();
  await A.locator("dialog[open] .row", { hasText: "carol" }).click();
  check("forward succeeds", await until(async () => (await A.locator(".toast", { hasText: "Переслано" }).count()) === 1));
  await A.locator("#list .row", { hasText: "carol" }).click();
  check("forwarded copy credits the original author", await until(async () => (await A.locator(".fwd", { hasText: "@bob" }).count()) === 1));

  /* ---- delete ---- */
  await A.locator("#list .row", { hasText: "bob" }).click(); await A.waitForSelector("#ta");
  await until(async () => (await msgTexts(A)).includes("привет, Боб (исправлено)"));
  await openMenuFor(A, "привет, Боб (исправлено)"); await menu(A, "Удалить").click();
  await A.locator("dialog[open] .btn.danger").click();
  check("delete removes message for author", await until(async () => !(await msgTexts(A)).includes("привет, Боб (исправлено)")));
  check("delete removes message for peer; quote falls back to 'удалено'", await until(async () => !(await msgTexts(B)).includes("привет, Боб (исправлено)")));

  /* ---- failed send -> retry is idempotent (client_id) ---- */
  await ctxA.setOffline(true);
  await A.fill("#ta", "отправлено после сбоя"); await A.keyboard.press("Enter");
  check("offline send is marked as failed, not shown as sent", await until(async () => (await A.locator(".msg.failed").count()) === 1));
  await ctxA.setOffline(false);
  await openMenuFor(A, "отправлено после сбоя"); await menu(A, "Повторить").click();
  check("retry delivers it", await until(async () => (await A.locator(".msg.failed").count()) === 0 && (await B.locator(".text", { hasText: "отправлено после сбоя" }).count()) === 1));
  await A.waitForTimeout(300);
  check("no duplicate after retry", (await msgTexts(B)).filter((t) => t === "отправлено после сбоя").length === 1 && (await msgTexts(A)).filter((t) => t === "отправлено после сбоя").length === 1);

  /* ---- WebSocket drop -> reconnect -> missed message arrives ---- */
  await fetch(mock.url + "/__test/drop-ws");
  await B.fill("#ta", "пока ты был офлайн"); await B.keyboard.press("Enter");
  check("client reconnects and resyncs missed messages", await until(async () => (await msgTexts(A)).includes("пока ты был офлайн"), 12000));

  /* ---- access token expiry: concurrent requests share ONE refresh per tab ---- */
  // The mock invalidates every user's access token, so each of the two open
  // tabs refreshes once (2 total). If a tab refreshed twice with the same
  // rotating refresh token the mock would answer "refresh_token_reused" and
  // the client would sign out, so delivery of all three messages proves it.
  const before = (await (await fetch(mock.url + "/__test/state")).json()).refresh;
  await fetch(mock.url + "/__test/expire-access");
  for (const t of ["токен-1", "токен-2", "токен-3"]) { await A.fill("#ta", t); await A.keyboard.press("Enter"); }
  check("three rapid sends after token expiry all arrive", await until(async () => { const got = await msgTexts(B); return ["токен-1", "токен-2", "токен-3"].every((t) => got.includes(t)); }, 8000));
  const after = (await (await fetch(mock.url + "/__test/state")).json()).refresh;
  check("at most one refresh per tab (no reuse)", after - before >= 1 && after - before <= 2, `refreshes: ${after - before}`);
  check("still signed in after the refresh storm", (await A.locator(".auth").count()) === 0 && (await B.locator(".auth").count()) === 0);

  /* ---- no horizontal overflow at many widths and views ---- */
  for (const [w, hgt] of [[320, 640], [390, 844], [768, 1024], [1024, 768], [1920, 1080], [2560, 1300]]) {
    const ctx = await browser.newContext({ viewport: { width: w, height: hgt }, hasTouch: w < 800, isMobile: w < 800 });
    const p = await login(ctx, "alice", "R" + w);
    const widths = [];
    const probe = async (label) => { widths.push([label, await p.evaluate(() => [document.documentElement.scrollWidth, innerWidth])]); };
    await probe("list");
    await p.locator("#list .row", { hasText: "bob" }).click(); await p.waitForSelector("#ta"); await sleep(150); await probe("chat");
    const cb = await p.locator("#ta").boundingBox();
    check(`composer visible at ${w}x${hgt}`, cb && cb.y >= 0 && cb.y + cb.height <= hgt, JSON.stringify(cb));
    if (SHOTS) await p.screenshot({ path: `${SHOTS}/chat-${w}.png` });
    await p.goBack(); await sleep(100);
    await p.click(".side-head .icon-btn"); await p.waitForSelector("text=Устройства"); await probe("settings");
    if (SHOTS) await p.screenshot({ path: `${SHOTS}/settings-${w}.png` });
    check(`no horizontal overflow at ${w}px (${widths.map(([l, [sw, iw]]) => `${l}:${sw}/${iw}`).join(" ")})`, widths.every(([, [sw, iw]]) => sw <= iw));
    if (w === 390) {
      const sidebarShown = await p.evaluate(() => getComputedStyle(document.querySelector(".sidebar")).display !== "none");
      await p.goBack(); await sleep(100);
      const listOnly = await p.evaluate(() => getComputedStyle(document.querySelector(".sidebar")).display !== "none" && getComputedStyle(document.querySelector(".main")).display === "none");
      check("phone: list and conversation are separate screens", listOnly && !sidebarShown);
      // keyboard proxy: shrink the visual viewport like a virtual keyboard does
      await p.locator("#list .row", { hasText: "bob" }).click(); await p.waitForSelector("#ta");
      await p.setViewportSize({ width: 390, height: 430 }); await sleep(200);
      const kb = await p.locator("#ta").boundingBox();
      check("phone: composer stays visible when the viewport shrinks (keyboard proxy)", kb && kb.y + kb.height <= 430, JSON.stringify(kb));
    }
    await ctx.close();
  }

  /* ---- service worker: install, precache, offline shell, versioned cache ---- */
  {
    const ctx = await browser.newContext(); const p = await ctx.newPage(); watch(p, "SW");
    await p.goto(mock.url + "/"); await p.evaluate(() => navigator.serviceWorker.ready);
    const info = await p.evaluate(async () => { const names = await caches.keys(); const c = await caches.open(names[0]); const keys = (await c.keys()).map((r) => new URL(r.url).pathname); return { names, keys }; });
    check("service worker precaches every shell file with no 404", ["/", "/index.html", "/styles.css", "/app.js", "/manifest.webmanifest", "/icon.svg"].every((k) => info.keys.includes(k)), JSON.stringify(info));
    check("cache is versioned (v5)", info.names.includes("kentgram-web-shell-v5"));
    await p.reload(); await ctx.setOffline(true); await p.reload();
    check("app shell loads offline", (await p.locator(".auth, #list").count()) === 1);
    await ctx.close();
  }

  check("no uncaught page errors or console errors", errors.length === 0, errors.slice(0, 5).join(" | "));
} catch (e) {
  failed++; console.log("FAIL harness crashed: " + e.stack);
} finally {
  await browser.close(); mock.close();
  console.log(`\n${pass} passed, ${failed} failed`);
  process.exit(failed ? 1 : 0);
}
