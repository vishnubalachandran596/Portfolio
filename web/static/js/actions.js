// Small page behaviours that used to be inline on* attributes. The site's
// script policy has no 'unsafe-inline', so markup opts in with data-
// attributes and this file (served from our own origin) does the work.
//   data-confirm="Text"        on a <form>: ask before submitting
//   data-select-on-click       on a field: select its text when clicked
//   data-autosubmit            on a field: submit its form when changed
//   data-select-all=".sel"     on a checkbox: tick/untick every match
(function () {
  document.addEventListener("submit", function (e) {
    var msg = e.target && e.target.getAttribute && e.target.getAttribute("data-confirm");
    if (msg && !window.confirm(msg)) e.preventDefault();
  }, true);
  document.addEventListener("click", function (e) {
    var t = e.target;
    if (t && t.hasAttribute && t.hasAttribute("data-select-on-click") && t.select) t.select();
  });
  document.addEventListener("change", function (e) {
    var t = e.target;
    if (!t || !t.hasAttribute) return;
    if (t.hasAttribute("data-autosubmit") && t.form) t.form.submit();
    var sel = t.getAttribute("data-select-all");
    if (sel) document.querySelectorAll(sel).forEach(function (c) { c.checked = t.checked; });
  });
})();
