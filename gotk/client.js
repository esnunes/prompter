// gotk thin client — connects WebSocket, loads WASM, scans gotk-* attributes,
// routes commands (WASM local or WebSocket), applies instructions.
(function() {
  "use strict";

  var ws = null;
  var refCounter = 0;
  var pendingLoading = {};  // ref -> {el, originalText}
  var registeredFns = {};
  var reconnectAttempt = 0;
  var reconnectDelays = [0, 2000, 5000, 10000, 30000];
  var boundElements = new WeakSet();

  // WASM state
  var wasmReady = false;
  var localCmds = null; // Set of command names registered in WASM
  var wasmExec = null;  // function(cmd, payloadJSON) => resultJSON

  // Public API
  window.gotk = {
    register: function(name, fn) {
      registeredFns[name] = fn;
    }
  };

  function nextRef() {
    refCounter++;
    return String(refCounter);
  }

  // --- WASM loading ---

  function initWASM() {
    if (typeof WebAssembly === "undefined") return;

    // TinyGo WASM requires wasm_exec.js to be loaded first (provides Go class)
    if (typeof Go === "undefined") return;

    var go = new Go();
    fetch("/gotk/app.wasm")
      .then(function(resp) {
        if (!resp.ok) return null;
        return WebAssembly.instantiateStreaming(resp, go.importObject);
      })
      .then(function(result) {
        if (!result) return;
        go.run(result.instance);

        // TinyGo WASM exposes functions via js.Global().Set() from Go side.
        // The WASM entry point registers listCommands/execCommand on window.
        if (typeof window.__gotk_listCommands === "function" &&
            typeof window.__gotk_execCommand === "function") {
          var cmdsJSON = window.__gotk_listCommands();
          var cmds = JSON.parse(cmdsJSON);
          localCmds = new Set(cmds);
          wasmExec = window.__gotk_execCommand;
          wasmReady = true;
        }
      })
      .catch(function(err) {
        console.warn("gotk: WASM load failed, all commands will route to server:", err);
      });
  }

  // --- Command dispatch ---

  function dispatchCommand(cmd, payload, ref) {
    // Try WASM first
    if (wasmReady && localCmds && localCmds.has(cmd)) {
      var payloadJSON = JSON.stringify(payload || {});
      var resultJSON = wasmExec(cmd, payloadJSON);
      var result;
      try { result = JSON.parse(resultJSON); } catch(_) { return; }

      // Apply immediate instructions (skip "cmd" ops — handled via async below)
      if (result.ins) {
        for (var i = 0; i < result.ins.length; i++) {
          if (result.ins[i].op !== "cmd") {
            applyInstruction(result.ins[i]);
          }
        }
      }

      // Dispatch async server commands
      if (result.async) {
        for (var j = 0; j < result.async.length; j++) {
          var ac = result.async[j];
          send(ac.Cmd || ac.cmd, ac.Payload || ac.payload || {}, nextRef());
        }
      }

      // Restore loading state immediately for WASM commands (synchronous)
      if (ref && pendingLoading[ref]) {
        var info = pendingLoading[ref];
        info.el.textContent = info.originalText;
        info.el.disabled = false;
        delete pendingLoading[ref];
      }
      return;
    }

    // Fall back to WebSocket
    send(cmd, payload, ref);
  }

  // --- WebSocket ---

  function connect() {
    var proto = location.protocol === "https:" ? "wss:" : "ws:";
    var url = proto + "//" + location.host + "/ws";
    ws = new WebSocket(url);

    ws.onopen = function() {
      reconnectAttempt = 0;
      document.body.classList.add("gotk-connected");
      document.body.classList.remove("gotk-disconnected");
    };

    ws.onclose = function() {
      document.body.classList.remove("gotk-connected");
      document.body.classList.add("gotk-disconnected");
      // Restore any buttons stuck in loading state
      Object.keys(pendingLoading).forEach(function(ref) {
        var info = pendingLoading[ref];
        if (info.el) {
          info.el.textContent = info.originalText;
          info.el.disabled = false;
        }
      });
      pendingLoading = {};
      scheduleReconnect();
    };

    ws.onerror = function() {};

    ws.onmessage = function(e) {
      var msg;
      try { msg = JSON.parse(e.data); } catch(_) { return; }
      handleResponse(msg);
    };
  }

  function scheduleReconnect() {
    var delay = reconnectDelays[Math.min(reconnectAttempt, reconnectDelays.length - 1)];
    reconnectAttempt++;
    setTimeout(function() {
      connect();
    }, delay);
  }

  function send(cmd, payload, ref) {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    ws.send(JSON.stringify({ cmd: cmd, payload: payload || {}, ref: ref || "" }));
  }

  // --- Response handling ---

  function handleResponse(msg) {
    // Restore gotk-loading element
    if (msg.ref && pendingLoading[msg.ref]) {
      var info = pendingLoading[msg.ref];
      info.el.textContent = info.originalText;
      info.el.disabled = false;
      delete pendingLoading[msg.ref];
    }

    if (msg.error) {
      console.warn("gotk:", msg.error);
    }

    if (msg.ins) {
      for (var i = 0; i < msg.ins.length; i++) {
        applyInstruction(msg.ins[i]);
      }
    }
  }

  // --- HTML sanitization (defense-in-depth) ---

  // Configure DOMPurify to allow gotk-* attributes
  if (typeof DOMPurify !== "undefined") {
    DOMPurify.addHook("uponSanitizeAttribute", function(node, data) {
      if (data.attrName.indexOf("gotk-") === 0) {
        data.forceKeepAttr = true;
      }
    });
  }

  function sanitizeHTML(html) {
    if (typeof DOMPurify !== "undefined") return DOMPurify.sanitize(html);
    return html;
  }

  // --- Instruction application ---

  function applyInstruction(ins) {
    switch (ins.op) {
      case "html":
        applyHTML(ins);
        break;
      case "template":
        applyTemplate(ins);
        break;
      case "populate":
        applyPopulate(ins);
        break;
      case "navigate":
        applyNavigate(ins);
        break;
      case "attr-set":
        var el = document.querySelector(ins.target);
        if (el) el.setAttribute(ins.attr, ins.value || "");
        else console.warn("gotk: target not found:", ins.target);
        break;
      case "attr-remove":
        var el2 = document.querySelector(ins.target);
        if (el2) el2.removeAttribute(ins.attr);
        else console.warn("gotk: target not found:", ins.target);
        break;
      case "set-value":
        var el3 = document.querySelector(ins.target);
        if (el3) el3.value = ins.value || "";
        else console.warn("gotk: target not found:", ins.target);
        break;
      case "dispatch":
        var el4 = document.querySelector(ins.target);
        if (el4) el4.dispatchEvent(new CustomEvent(ins.event, { detail: ins.detail || {}, bubbles: true }));
        else console.warn("gotk: target not found:", ins.target);
        break;
      case "focus":
        var el5 = document.querySelector(ins.target);
        if (el5) el5.focus();
        else console.warn("gotk: target not found:", ins.target);
        break;
      case "exec":
        var fn = registeredFns[ins.name];
        if (fn) fn(ins.args || {});
        else console.warn("gotk: unknown function:", ins.name);
        break;
      case "cmd":
        send(ins.cmd, ins.payload);
        break;
      default:
        console.warn("gotk: unknown instruction op:", ins.op);
    }
  }

  function applyHTML(ins) {
    var target = document.querySelector(ins.target);
    if (!target) { console.warn("gotk: target not found:", ins.target); return; }

    var mode = ins.mode || "replace";
    if (mode === "remove") {
      target.remove();
      return;
    }
    var safeHTML = sanitizeHTML(ins.html);
    if (mode === "replace") {
      target.innerHTML = safeHTML;
      scanElement(target);
    } else if (mode === "append") {
      var tmp = document.createElement("div");
      tmp.innerHTML = safeHTML;
      while (tmp.firstChild) {
        var child = tmp.firstChild;
        target.appendChild(child);
        if (child.nodeType === 1) scanElement(child);
      }
    } else if (mode === "prepend") {
      var tmp2 = document.createElement("div");
      tmp2.innerHTML = safeHTML;
      var first = target.firstChild;
      while (tmp2.firstChild) {
        var child2 = tmp2.firstChild;
        target.insertBefore(child2, first);
        if (child2.nodeType === 1) scanElement(child2);
      }
    }
  }

  function applyTemplate(ins) {
    var source = document.querySelector(ins.source);
    var target = document.querySelector(ins.target);
    if (!source || !target) {
      console.warn("gotk: template source/target not found:", ins.source, ins.target);
      return;
    }
    var clone = source.content.cloneNode(true);
    target.innerHTML = "";
    target.appendChild(clone);
    scanElement(target);
  }

  function applyPopulate(ins) {
    var target = document.querySelector(ins.target);
    if (!target) { console.warn("gotk: target not found:", ins.target); return; }
    var data = ins.data || {};
    for (var key in data) {
      var el = target.querySelector('[name="' + key + '"]');
      if (el) el.value = data[key];
    }
  }

  function applyNavigate(ins) {
    if (ins.url) {
      history.pushState(null, "", ins.url);
    }
    if (ins.target && ins.html) {
      var target = document.querySelector(ins.target);
      if (target) {
        target.innerHTML = sanitizeHTML(ins.html);
        scanElement(target);
      }
    }
  }

  // --- Payload collection ---

  function collectPayload(el) {
    var payload = {};

    // gotk-payload (lowest priority)
    var payloadJSON = el.getAttribute("gotk-payload");
    if (payloadJSON) {
      try { payload = JSON.parse(payloadJSON); } catch(_) {}
    }

    // gotk-collect (middle priority)
    var collectSel = el.getAttribute("gotk-collect");
    if (collectSel) {
      var container = document.querySelector(collectSel);
      if (container) {
        var named = container.querySelectorAll("[name]");
        for (var i = 0; i < named.length; i++) {
          var input = named[i];
          var name = input.getAttribute("name");
          if (input.type === "checkbox") {
            if (payload[name] === undefined) payload[name] = [];
            if (input.checked) {
              if (Array.isArray(payload[name])) payload[name].push(input.value);
            }
          } else if (input.type === "radio") {
            if (input.checked) payload[name] = input.value;
          } else if (input.tagName === "SELECT" && input.multiple) {
            payload[name] = Array.from(input.selectedOptions).map(function(o) { return o.value; });
          } else {
            payload[name] = input.value;
          }
        }
      }
    }

    // gotk-val-* (highest priority)
    var attrs = el.attributes;
    for (var j = 0; j < attrs.length; j++) {
      if (attrs[j].name.indexOf("gotk-val-") === 0) {
        var key = attrs[j].name.substring(9); // strip "gotk-val-"
        payload[key] = attrs[j].value;
      }
    }

    return payload;
  }

  // --- DOM scanning ---

  function scanElement(root) {
    if (!root || !root.querySelectorAll) return;

    // Scan root itself
    bindElement(root);

    // Scan descendants
    var els = root.querySelectorAll("[gotk-click],[gotk-navigate],[gotk-on],[gotk-input]");
    for (var i = 0; i < els.length; i++) {
      bindElement(els[i]);
    }
  }

  function bindElement(el) {
    if (boundElements.has(el)) return;

    // gotk-click
    var clickCmd = el.getAttribute("gotk-click");
    if (clickCmd) {
      boundElements.add(el);
      el.addEventListener("click", function(e) {
        e.preventDefault();
        e.stopPropagation();
        var cmd = el.getAttribute("gotk-click");
        var payload = collectPayload(el);
        var ref = nextRef();

        // gotk-loading
        var loadingText = el.getAttribute("gotk-loading");
        if (loadingText) {
          pendingLoading[ref] = { el: el, originalText: el.textContent };
          el.textContent = loadingText;
          el.disabled = true;
        }

        dispatchCommand(cmd, payload, ref);
      });
    }

    // gotk-navigate
    if (el.hasAttribute("gotk-navigate")) {
      boundElements.add(el);
      el.addEventListener("click", function(e) {
        e.preventDefault();
        var url = el.getAttribute("href") || el.getAttribute("gotk-navigate");
        if (url) {
          dispatchCommand("navigate", { url: url }, nextRef());
        }
      });
    }

    // gotk-on="event:cmd" — binds arbitrary DOM events to commands
    var onAttr = el.getAttribute("gotk-on");
    if (onAttr) {
      boundElements.add(el);
      var parts = onAttr.split(":");
      if (parts.length >= 2) {
        var eventName = parts[0];
        var onCmd = parts.slice(1).join(":"); // allow colons in cmd name
        el.addEventListener(eventName, function(e) {
          var payload = collectPayload(el);
          // Include key event metadata under _event
          if (e instanceof KeyboardEvent) {
            payload._event = {
              key: e.key,
              code: e.code,
              shiftKey: e.shiftKey,
              ctrlKey: e.ctrlKey,
              altKey: e.altKey,
              metaKey: e.metaKey,
              isComposing: e.isComposing
            };
          }
          dispatchCommand(onCmd, payload, nextRef());
        });
      }
    }

    // gotk-input="cmd" — sends command on input event with optional debounce
    var inputCmd = el.getAttribute("gotk-input");
    if (inputCmd) {
      boundElements.add(el);
      var debounceMs = parseInt(el.getAttribute("gotk-debounce") || "0", 10);
      var debounceTimer = null;
      el.addEventListener("input", function() {
        var payload = collectPayload(el);
        payload.value = el.value;
        if (debounceMs > 0) {
          clearTimeout(debounceTimer);
          debounceTimer = setTimeout(function() {
            dispatchCommand(inputCmd, payload, nextRef());
          }, debounceMs);
        } else {
          dispatchCommand(inputCmd, payload, nextRef());
        }
      });
    }
  }

  // --- Popstate ---

  window.addEventListener("popstate", function() {
    dispatchCommand("navigate", { url: location.pathname + location.search }, nextRef());
  });

  // --- Init ---

  document.addEventListener("DOMContentLoaded", function() {
    scanElement(document.body);
    initWASM();
    connect();
  });
})();
