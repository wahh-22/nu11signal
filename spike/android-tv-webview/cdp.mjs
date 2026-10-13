#!/usr/bin/env node
// Evaluates one JavaScript expression in the spike's WebView over the Chrome
// DevTools Protocol and prints the JSON result. Promises are awaited.
//
//   adb forward tcp:9222 localabstract:webview_devtools_remote_<pid>
//   node cdp.mjs 'document.title'
//   node cdp.mjs --reload
//
// CDP_PORT overrides the forwarded port (default 9222). CDP_TARGET picks the
// page whose URL contains it (default music.apple.com): MusicKit's sign-in
// window is a second page target.
const port = process.env.CDP_PORT ?? "9222";
const wanted = process.env.CDP_TARGET ?? "music.apple.com";
const arg = process.argv[2];
if (!arg) {
  console.error("usage: node cdp.mjs <expression> | --reload");
  process.exit(2);
}

const targets = await (await fetch(`http://127.0.0.1:${port}/json`)).json();
const page = targets.find((t) => t.type === "page" && t.url.includes(wanted));
if (!page) throw new Error(`no page target with a URL containing ${wanted}`);

const ws = new WebSocket(page.webSocketDebuggerUrl);
await new Promise((resolve, reject) => {
  ws.onopen = resolve;
  ws.onerror = reject;
});

let nextId = 1;
const pending = new Map();
ws.onmessage = (msg) => {
  let data;
  try {
    data = JSON.parse(msg.data);
  } catch {
    return; // not a CDP frame we can use
  }
  const done = pending.get(data.id);
  if (done) {
    pending.delete(data.id);
    done(data);
  }
};
const send = (method, params = {}) =>
  new Promise((resolve) => {
    const id = nextId++;
    pending.set(id, resolve);
    ws.send(JSON.stringify({ id, method, params }));
  });

let reply;
if (arg === "--reload") {
  reply = await send("Page.reload", { ignoreCache: true });
} else {
  reply = await send("Runtime.evaluate", {
    expression: arg,
    awaitPromise: true,
    returnByValue: true,
  });
}
console.log(JSON.stringify(reply.result?.result?.value ?? reply.result ?? reply.error, null, 2));
ws.close();
