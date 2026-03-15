// Scroll to first question if present, otherwise scroll to bottom
function scrollConversation() {
  var c = document.getElementById("conversation");
  var q = document.getElementById("question-form");
  if (q) {
    // Scroll to the assistant message preceding the questions so the user
    // sees the full context (title + question text), not just the options.
    var target = q.previousElementSibling || q;
    target.scrollIntoView({ behavior: "smooth", block: "start" });
  } else if (c) {
    c.scrollTop = c.scrollHeight;
  }
}

// Update elapsed timers for processing indicators
function updateElapsedTimers() {
  var els = document.querySelectorAll("[data-started-at]");
  els.forEach(function (el) {
    var startedAt = parseInt(el.getAttribute("data-started-at"), 10);
    if (!startedAt) return;
    var timer = el.querySelector(".elapsed-timer");
    if (!timer) return;
    var elapsed = Math.floor(Date.now() / 1000) - startedAt;
    if (elapsed < 0) elapsed = 0;
    var mins = Math.floor(elapsed / 60);
    var secs = elapsed % 60;
    timer.textContent =
      mins > 0 ? "(" + mins + "m " + secs + "s)" : "(" + secs + "s)";
  });
}

setInterval(updateElapsedTimers, 1000);

// Render markdown in assistant message bubbles and revision content
function renderMarkdown() {
  var bubbles = document.querySelectorAll(
    ".message-assistant .message-bubble:not([data-md-rendered]), .revision-content:not([data-md-rendered])"
  );
  bubbles.forEach(function (el) {
    if (typeof DOMPurify !== "undefined" && typeof marked !== "undefined") {
      el.innerHTML = DOMPurify.sanitize(marked.parse(el.textContent));
      el.setAttribute("data-md-rendered", "");
    }
  });
}

// Post-navigation page initialization (markdown, scroll, focus)
function initPage() {
  renderMarkdown();

  // Skip auto-scroll if URL has a hash fragment (e.g., #revision-3)
  if (window.location.hash) return;

  var c = document.getElementById("conversation");
  var q = document.getElementById("question-form");
  if (q) {
    var target = q.previousElementSibling || q;
    target.scrollIntoView({ behavior: "instant", block: "start" });
  } else if (c) {
    c.scrollTo({ top: c.scrollHeight, behavior: "instant" });
  }

  // Focus textarea unless questionnaire is showing or on mobile
  if (!q && window.innerWidth >= 769) {
    var textarea = document.querySelector(".chat-form textarea");
    if (textarea && !textarea.disabled) {
      textarea.focus();
    }
  }
}

// Intercept internal link clicks for SPA navigation
document.addEventListener("click", function (e) {
  var link = e.target.closest("a[href]");
  if (!link) return;

  // Skip if modifier keys (user wants new tab)
  if (e.ctrlKey || e.metaKey || e.shiftKey || e.altKey) return;

  // Skip external links and target="_blank"
  if (link.target === "_blank") return;

  var href = link.getAttribute("href");
  if (!href) return;

  // Skip anchor-only links
  if (href.charAt(0) === "#") return;

  // Skip absolute external URLs
  if (/^https?:\/\//.test(href)) return;

  // Skip gotk-click elements (gotk handles them)
  if (link.hasAttribute("gotk-click")) return;

  // Skip gotk-navigate elements (client.js handles them)
  if (link.hasAttribute("gotk-navigate")) return;

  e.preventDefault();
  if (window.gotk && gotk.navigate) gotk.navigate(href);
});

// Enter-to-send: submit chat form on Enter, newline on Shift+Enter
document.addEventListener("keydown", function (e) {
  if (e.key !== "Enter") return;
  var textarea = e.target;
  if (textarea.id !== "message-input") return;

  // Allow Shift+Enter to insert newline (default behavior)
  if (e.shiftKey) return;

  // Don't submit during IME composition (CJK input)
  if (e.isComposing || e.keyCode === 229) return;

  e.preventDefault();

  // Don't submit empty/whitespace-only messages
  if (textarea.value.trim() === "") return;

  // Don't submit if textarea is disabled (processing in progress)
  if (textarea.disabled) return;

  var sendBtn = document.getElementById("send-btn");
  if (sendBtn && !sendBtn.disabled) {
    sendBtn.click();
  }
});

document.addEventListener("DOMContentLoaded", function () {
  initPage();
});

// Highlight the active sidebar item based on the current URL path
function highlightCurrentSidebar() {
  var items = document.querySelectorAll(".prompt-list-item");
  var path = window.location.pathname;
  items.forEach(function (item) {
    var link = item.querySelector("a[href]");
    if (link && path === link.getAttribute("href")) {
      item.classList.add("prompt-list-item-active");
    } else {
      item.classList.remove("prompt-list-item-active");
    }
  });
}

// Register gotk exec functions for use by server commands
document.addEventListener("DOMContentLoaded", function () {
  if (window.gotk) {
    gotk.register("initPage", initPage);
    gotk.register("scrollConversation", scrollConversation);
    gotk.register("renderMarkdown", renderMarkdown);
    gotk.register("updateElapsedTimers", updateElapsedTimers);
    gotk.register("highlightCurrentSidebar", highlightCurrentSidebar);
  }
});
