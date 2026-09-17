(function () {
  "use strict";

  var navToggle = document.getElementById("nav-toggle");
  var nav = document.getElementById("primary-nav");
  if (navToggle && nav) {
    navToggle.addEventListener("click", function () {
      var open = nav.classList.toggle("open");
      navToggle.setAttribute("aria-expanded", open ? "true" : "false");
    });
  }

  var slotInput = document.getElementById("slot-input");
  var contact = document.getElementById("booking-contact");
  var selectedLabel = document.getElementById("selected-slot-label");
  if (slotInput && contact) {
    document.querySelectorAll(".slot-btn").forEach(function (btn) {
      btn.addEventListener("click", function () {
        if (btn.disabled) return;
        document.querySelectorAll(".slot-btn.selected").forEach(function (b) {
          b.classList.remove("selected");
        });
        btn.classList.add("selected");
        slotInput.value = btn.getAttribute("data-slot");
        if (selectedLabel) selectedLabel.textContent = btn.getAttribute("data-label");
        contact.hidden = false;
        contact.scrollIntoView({ behavior: "smooth", block: "center" });
      });
    });
  }
})();
