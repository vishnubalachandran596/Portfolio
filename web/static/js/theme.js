// Shared theme init for pages that don't load main.js (the admin
// console) — applies whatever the visitor already picked on the public
// site (same "theme" localStorage key) and wires up a #theme-toggle
// button if the page has one, so admin doesn't silently fall back to
// the OS theme while the public site respects an explicit choice.
(function () {
  "use strict";
  function currentTheme() {
    try {
      return localStorage.getItem("theme");
    } catch (e) {
      return null;
    }
  }
  function applyTheme(theme) {
    if (theme === "light" || theme === "dark") {
      document.documentElement.setAttribute("data-theme", theme);
    } else {
      document.documentElement.removeAttribute("data-theme");
    }
  }
  applyTheme(currentTheme());

  document.addEventListener("DOMContentLoaded", function () {
    var themeToggle = document.getElementById("theme-toggle");
    if (!themeToggle) return;
    themeToggle.addEventListener("click", function () {
      var prefersDark = window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches;
      var effective = currentTheme() || (prefersDark ? "dark" : "light");
      var next = effective === "dark" ? "light" : "dark";
      applyTheme(next);
      try {
        localStorage.setItem("theme", next);
      } catch (e) {}
    });
  });
})();
