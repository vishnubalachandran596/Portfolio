(function () {
  "use strict";

  var path = window.location.pathname;
  var links = document.querySelectorAll(".admin-sidebar-nav a[data-admin-path]");
  var best = null;
  links.forEach(function (link) {
    var target = link.getAttribute("data-admin-path");
    if (path === target || (target !== "/admin" && path.indexOf(target + "/") === 0)) {
      if (!best || target.length > best.getAttribute("data-admin-path").length) {
        best = link;
      }
    }
  });
  if (best) best.classList.add("active");

  var toggle = document.getElementById("admin-sidebar-toggle");
  var sidebar = document.getElementById("admin-sidebar");
  var backdrop = document.getElementById("admin-sidebar-backdrop");

  function closeSidebar() {
    sidebar.classList.remove("open");
    backdrop.hidden = true;
    toggle.setAttribute("aria-expanded", "false");
  }

  if (toggle && sidebar && backdrop) {
    toggle.addEventListener("click", function () {
      var open = sidebar.classList.toggle("open");
      backdrop.hidden = !open;
      toggle.setAttribute("aria-expanded", String(open));
    });
    backdrop.addEventListener("click", closeSidebar);
  }
})();
