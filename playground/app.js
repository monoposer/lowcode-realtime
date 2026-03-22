(function () {
  const $ = (id) => document.getElementById(id);
  const logEl = $("log");
  let ws = null;
  const STORAGE_BACKEND = "playground_backend_base";

  function normalizeBackend(s) {
    s = (s || "").trim().replace(/\/$/, "");
    return s || "http://localhost:8080";
  }

  function httpOriginToWsOrigin(httpOrigin) {
    try {
      const u = new URL(httpOrigin);
      u.protocol = u.protocol === "https:" ? "wss:" : "ws:";
      return u.origin;
    } catch (e) {
      return "ws://localhost:8080";
    }
  }

  function defaultWsFromBackend() {
    return httpOriginToWsOrigin(normalizeBackend($("backendBase").value)) + "/ws?user_id=demo";
  }

  function apiEventURL() {
    return normalizeBackend($("backendBase").value) + "/api/event";
  }

  /** Turn relative paths like "/ws?user_id=demo" into absolute ws://... using the HTTP base. */
  function absolutizeWebSocketURL(raw) {
    let url = (raw || "").trim();
    if (!url) return "";
    if (/^wss?:\/\//i.test(url)) return url;
    const base = normalizeBackend($("backendBase").value);
    try {
      const h = new URL(base);
      const origin = (h.protocol === "https:" ? "wss:" : "ws:") + "//" + h.host;
      return origin + (url.startsWith("/") ? url : "/" + url);
    } catch (e) {
      return url;
    }
  }

  /** Merge optional JWT into the WebSocket URL as `token` query param; drops `user_id` when token is set. */
  function buildWebSocketURL() {
    let url = absolutizeWebSocketURL($("wsUrl").value);
    const tok = $("wsToken").value.trim();
    if (!tok) {
      return url;
    }
    try {
      const u = new URL(url);
      u.searchParams.set("token", tok);
      u.searchParams.delete("user_id");
      return u.toString();
    } catch (e) {
      throw new Error("Invalid WebSocket URL: " + (e.message || String(e)));
    }
  }

  try {
    const saved = localStorage.getItem(STORAGE_BACKEND);
    if (saved) $("backendBase").value = saved;
  } catch (e) {}

  $("backendBase").addEventListener("change", function () {
    try {
      localStorage.setItem(STORAGE_BACKEND, $("backendBase").value.trim());
    } catch (e) {}
  });

  $("btnSyncWs").onclick = function () {
    $("wsUrl").value = defaultWsFromBackend();
  };

  $("wsUrl").placeholder = defaultWsFromBackend();
  if (!$("wsUrl").value) $("wsUrl").value = defaultWsFromBackend();

  function log(line, kind) {
    const t = new Date().toISOString();
    const prefix = kind === "err" ? "[ERR] " : "";
    logEl.textContent += prefix + "[" + t + "] " + line + "\n";
    logEl.scrollTop = logEl.scrollHeight;
  }

  function setWsStatus(text, ok) {
    const el = $("wsStatus");
    el.textContent = text;
    el.className = "status " + (ok === true ? "ok" : ok === false ? "err" : "");
  }

  $("btnConnect").onclick = function () {
    if (ws && ws.readyState === WebSocket.OPEN) {
      log("Already connected; disconnect first.");
      return;
    }
    let url;
    try {
      url = buildWebSocketURL();
    } catch (e) {
      log(e.message || String(e), "err");
      return;
    }
    if (!url) {
      log("WebSocket URL is required", "err");
      return;
    }
    let listen;
    try {
      listen = JSON.parse($("listen").value || '["broadcast"]');
    } catch (e) {
      log("Invalid JSON in listen: " + e.message, "err");
      return;
    }
    const channel = $("channel").value.trim() || "room1";

    ws = new WebSocket(url);
    $("btnConnect").disabled = true;
    setWsStatus("Connecting…", null);

    ws.onopen = function () {
      setWsStatus("Connected", true);
      $("btnDisconnect").disabled = false;
      const sub = JSON.stringify({
        type: "subscribe",
        channel: channel,
        listen: listen,
      });
      ws.send(sub);
      log("→ " + sub);
    };

    ws.onmessage = function (ev) {
      log("← " + (typeof ev.data === "string" ? ev.data : String(ev.data)));
    };

    ws.onerror = function () {
      log("WebSocket error (see close code below; browser does not expose HTTP body for failed upgrade)", "err");
      setWsStatus("Error", false);
    };

    ws.onclose = function (ev) {
      setWsStatus("Disconnected", false);
      $("btnConnect").disabled = false;
      $("btnDisconnect").disabled = true;
      var line =
        "WebSocket closed code=" +
        ev.code +
        " reason=" +
        (ev.reason || "(none)") +
        " wasClean=" +
        ev.wasClean;
      if (ev.code === 1006) {
        line +=
          " — abnormal closure: often wrong ws/wss URL, server down, or HTTP 401/400 instead of upgrade (missing user_id, or JWT required).";
      }
      log(line);
      ws = null;
    };
  };

  $("btnDisconnect").onclick = function () {
    if (ws) ws.close();
  };

  $("btnSendApi").onclick = async function () {
    const channel = $("apiChannel").value.trim();
    const event = $("apiEvent").value.trim() || "message";
    let payload;
    try {
      payload = JSON.parse($("apiPayload").value || "{}");
    } catch (e) {
      $("apiStatus").textContent = "Invalid payload JSON: " + e.message;
      $("apiStatus").className = "status err";
      return;
    }
    $("apiStatus").textContent = "Sending…";
    $("apiStatus").className = "status";
    const apiUrl = apiEventURL();
    try {
      const res = await fetch(apiUrl, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          channel: channel,
          event: event,
          payload: payload,
        }),
      });
      const text = await res.text();
      if (!res.ok) {
        $("apiStatus").textContent = "HTTP " + res.status + " " + text;
        $("apiStatus").className = "status err";
        log("API error: " + res.status + " " + text, "err");
        return;
      }
      $("apiStatus").textContent = "OK " + text;
      $("apiStatus").className = "status ok";
      log("API ← " + text);
    } catch (e) {
      $("apiStatus").textContent = String(e);
      $("apiStatus").className = "status err";
      log("API exception: " + e.message, "err");
    }
  };

  $("btnClear").onclick = function () {
    logEl.textContent = "";
  };
})();
