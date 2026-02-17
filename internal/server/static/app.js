(function () {
  function renderMarkdown(root) {
    var bubbles = (root || document).querySelectorAll(
      ".message-assistant .message-bubble:not([data-md-rendered]), .revision-content:not([data-md-rendered])"
    );
    bubbles.forEach(function (el) {
      el.innerHTML = marked.parse(el.textContent);
      el.setAttribute("data-md-rendered", "");
    });
  }

  document.addEventListener("DOMContentLoaded", function () {
    renderMarkdown();
  });

  document.addEventListener("htmx:afterSwap", function (e) {
    renderMarkdown(e.detail.target);
  });
})();
