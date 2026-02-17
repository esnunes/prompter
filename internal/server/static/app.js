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
