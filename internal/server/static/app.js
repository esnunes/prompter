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

(function () {
  function renderMarkdown(root) {
    var bubbles = (root || document).querySelectorAll(
      ".message-assistant .message-bubble:not([data-md-rendered]), .revision-content:not([data-md-rendered])"
    );
    bubbles.forEach(function (el) {
      el.innerHTML = DOMPurify.sanitize(marked.parse(el.textContent));
      el.setAttribute("data-md-rendered", "");
    });
  }

  document.addEventListener("DOMContentLoaded", function () {
    renderMarkdown();

    // Auto-scroll and focus on initial conversation page load.
    // Skip if URL has a hash fragment (e.g., #revision-3) to preserve
    // native anchor scroll from revision sidebar links.
    if (!window.location.hash && document.getElementById("conversation")) {
      // Scroll instantly (no animation) to avoid visual jank on load.
      var c = document.getElementById("conversation");
      var q = document.getElementById("question-form");
      if (q) {
        var target = q.previousElementSibling || q;
        target.scrollIntoView({ behavior: "instant", block: "start" });
      } else if (c) {
        c.scrollTo({ top: c.scrollHeight, behavior: "instant" });
      }

      // Focus textarea unless questionnaire is showing (textarea hidden)
      // or on mobile where keyboard would disrupt scroll position.
      if (!q && window.innerWidth >= 769) {
        var textarea = document.querySelector(".chat-form textarea");
        if (textarea && !textarea.disabled) {
          textarea.focus();
        }
      }
    }
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
})();

// Register gotk exec functions for use by server commands
document.addEventListener("DOMContentLoaded", function () {
  if (window.gotk) {
    gotk.register("scrollConversation", function () {
      if (typeof scrollConversation === "function") scrollConversation();
    });

    gotk.register("renderMarkdown", function () {
      var bubbles = document.querySelectorAll(
        ".message-assistant .message-bubble:not([data-md-rendered]), .revision-content:not([data-md-rendered])"
      );
      bubbles.forEach(function (el) {
        if (typeof DOMPurify !== "undefined" && typeof marked !== "undefined") {
          el.innerHTML = DOMPurify.sanitize(marked.parse(el.textContent));
          el.setAttribute("data-md-rendered", "");
        }
      });
    });

    gotk.register("updateElapsedTimers", function () {
      if (typeof updateElapsedTimers === "function") updateElapsedTimers();
    });
  }
});
