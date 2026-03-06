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

  // Validate question form before submission
  function validateQuestionForm(form) {
    var groups = form.querySelectorAll(".question-group");
    for (var i = 0; i < groups.length; i++) {
      var inputs = groups[i].querySelectorAll(
        'input[type="radio"], input[type="checkbox"]'
      );
      var anyChecked = false;
      var otherChecked = false;
      var otherInput = groups[i].querySelector(".other-input");

      for (var j = 0; j < inputs.length; j++) {
        if (inputs[j].checked) {
          anyChecked = true;
          if (inputs[j].value === "__other__") {
            otherChecked = true;
          }
        }
      }

      if (!anyChecked) {
        alert("Please select an option for each question.");
        return false;
      }

      if (otherChecked && otherInput && otherInput.value.trim() === "") {
        otherInput.focus();
        alert('Please enter a value for "Other".');
        return false;
      }
    }
    return true;
  }

  // Hide/show textarea when question blocks appear/disappear
  function updateMessageFormVisibility() {
    var messageForm = document.getElementById("message-form");
    var questionForm = document.getElementById("question-form");
    if (messageForm) {
      messageForm.style.display = questionForm ? "none" : "";
    }
  }

  document.addEventListener("DOMContentLoaded", function () {
    renderMarkdown();
  });

  document.addEventListener("htmx:afterSwap", function (e) {
    renderMarkdown(e.detail.target);
    updateMessageFormVisibility();
    updateElapsedTimers();

    // Only scroll when new content is appended to #conversation,
    // not on status poll swaps which would steal focus from buttons.
    var target = e.detail.target;
    if (target && target.id === "conversation") {
      scrollConversation();
    }
  });

  // Validate question forms before HTMX sends
  document.addEventListener("htmx:confirm", function (e) {
    var form = e.detail.elt;
    if (form.closest && form.closest(".question-block")) {
      if (!validateQuestionForm(form.closest("form") || form)) {
        e.preventDefault();
      }
    }
  });
})();
