(function () {
  "use strict";

  // --- First-visit splash (once per tab) -------------------------------
  var splash = document.getElementById("splash-loader");
  if (splash) {
    var seen = false;
    try {
      seen = sessionStorage.getItem("splash-seen") === "1";
    } catch (e) {}
    if (seen) {
      splash.classList.add("hide");
    } else {
      try {
        sessionStorage.setItem("splash-seen", "1");
      } catch (e) {}
      window.addEventListener("load", function () {
        setTimeout(function () {
          splash.classList.add("hide");
        }, 450);
      });
      // Never block the page for more than a moment even if load is slow.
      setTimeout(function () {
        splash.classList.add("hide");
      }, 2200);
    }
  }

  // --- Page transitions -------------------------------------------------
  // A light fade-out before navigating to an internal link, and a spinner
  // for anything slower than an instant cache hit, instead of a hard jump.
  var pageLoader = document.getElementById("page-loader");
  var pageLoaderMark = document.getElementById("page-loader-mark");
  var loaderTimer = null;
  document.addEventListener("click", function (e) {
    var link = e.target.closest && e.target.closest("a[href]");
    if (!link) return;
    var url;
    try {
      url = new URL(link.href, window.location.href);
    } catch (err) {
      return;
    }
    var isInternal = url.origin === window.location.origin;
    var isSamePage = url.pathname === window.location.pathname && url.search === window.location.search;
    if (!isInternal || isSamePage || link.target === "_blank" || link.hasAttribute("download") || e.metaKey || e.ctrlKey || e.shiftKey) {
      return;
    }
    // A short delay avoids a visible flash on an instant cache hit, but
    // short enough that a normal-speed navigation still shows the
    // branded indicator rather than only the browser's own loading bar.
    loaderTimer = setTimeout(function () {
      if (pageLoader) pageLoader.classList.add("active");
      if (pageLoaderMark) pageLoaderMark.classList.add("active");
    }, 60);
  });
  window.addEventListener("pageshow", function () {
    if (pageLoader) pageLoader.classList.remove("active");
    if (pageLoaderMark) pageLoaderMark.classList.remove("active");
    if (loaderTimer) clearTimeout(loaderTimer);
  });

  // --- Header dropdown menus (notifications, account) -------------------
  var headerDropdowns = document.querySelectorAll(".header-dropdown");
  function closeHeaderDropdowns(except) {
    headerDropdowns.forEach(function (panel) {
      if (panel !== except) {
        panel.hidden = true;
        var toggle = document.getElementById(panel.id.replace("-panel", "-toggle"));
        if (toggle) toggle.setAttribute("aria-expanded", "false");
      }
    });
  }
  function setupHeaderMenu(toggleId, panelId) {
    var toggle = document.getElementById(toggleId);
    var panel = document.getElementById(panelId);
    if (!toggle || !panel) return;
    toggle.addEventListener("click", function (e) {
      e.stopPropagation();
      var willOpen = panel.hidden;
      closeHeaderDropdowns(willOpen ? panel : null);
      panel.hidden = !willOpen;
      toggle.setAttribute("aria-expanded", String(willOpen));
    });
  }
  setupHeaderMenu("notif-toggle", "notif-panel");
  setupHeaderMenu("profile-toggle", "profile-panel");
  if (headerDropdowns.length) {
    document.addEventListener("click", function (e) {
      if (!e.target.closest(".header-menu")) closeHeaderDropdowns(null);
    });
    document.addEventListener("keydown", function (e) {
      if (e.key === "Escape") closeHeaderDropdowns(null);
    });
  }

  // --- Theme toggle (persisted, defaults to OS preference) -----------
  var themeToggle = document.getElementById("theme-toggle");
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
  if (themeToggle) {
    themeToggle.addEventListener("click", function () {
      var prefersDark = window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches;
      var effective = currentTheme() || (prefersDark ? "dark" : "light");
      var next = effective === "dark" ? "light" : "dark";
      applyTheme(next);
      try {
        localStorage.setItem("theme", next);
      } catch (e) {}
    });
  }

  // --- Cookie consent banner ------------------------------------------
  var cookieBanner = document.getElementById("cookie-banner");
  var cookieAccept = document.getElementById("cookie-accept");
  if (cookieBanner) {
    var consented = false;
    try {
      consented = localStorage.getItem("cookie-consent") === "1";
    } catch (e) {}
    if (!consented) cookieBanner.hidden = false;
    if (cookieAccept) {
      cookieAccept.addEventListener("click", function () {
        cookieBanner.hidden = true;
        try {
          localStorage.setItem("cookie-consent", "1");
        } catch (e) {}
      });
    }
  }

  // --- Scroll progress bar + back-to-top -------------------------------
  var progress = document.getElementById("scroll-progress");
  var backToTop = document.getElementById("back-to-top");
  function onScroll() {
    var doc = document.documentElement;
    var scrollTop = doc.scrollTop || document.body.scrollTop;
    var height = doc.scrollHeight - doc.clientHeight;
    if (progress) {
      var pct = height > 0 ? (scrollTop / height) * 100 : 0;
      progress.style.width = pct + "%";
    }
    if (backToTop) backToTop.hidden = scrollTop < 480;
  }
  document.addEventListener("scroll", onScroll, { passive: true });
  onScroll();
  if (backToTop) {
    backToTop.addEventListener("click", function () {
      window.scrollTo({ top: 0, behavior: "smooth" });
    });
  }

  // --- Password visibility toggle --------------------------------------
  document.querySelectorAll(".password-toggle").forEach(function (btn) {
    var input = document.getElementById(btn.getAttribute("data-target"));
    if (!input) return;
    btn.addEventListener("click", function () {
      var showing = input.type === "text";
      input.type = showing ? "password" : "text";
      btn.textContent = showing ? "Show" : "Hide";
      btn.setAttribute("aria-label", showing ? "Show password" : "Hide password");
    });
  });

  // --- Copy-to-clipboard buttons -----------------------------------------
  document.querySelectorAll(".copy-btn[data-copy]").forEach(function (btn) {
    var original = btn.textContent;
    btn.addEventListener("click", function () {
      var value = btn.getAttribute("data-copy");
      var done = function () {
        btn.textContent = "Copied!";
        btn.classList.add("copied");
        setTimeout(function () {
          btn.textContent = original;
          btn.classList.remove("copied");
        }, 1800);
      };
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(value).then(done, done);
      } else {
        done();
      }
    });
  });

  // --- Login / signup tab switch -----------------------------------------
  var authTabs = document.querySelectorAll(".auth-tab");
  var loginForm = document.getElementById("login-form");
  var signupForm = document.getElementById("signup-form");
  if (authTabs.length && loginForm && signupForm) {
    authTabs.forEach(function (tab) {
      tab.addEventListener("click", function () {
        var showSignup = tab.getAttribute("data-tab") === "signup";
        authTabs.forEach(function (t) {
          t.classList.toggle("active", t === tab);
          t.setAttribute("aria-selected", t === tab ? "true" : "false");
        });
        loginForm.hidden = showSignup;
        signupForm.hidden = !showSignup;
      });
    });
  }

  // --- Loading state on form submit buttons -----------------------------
  document.querySelectorAll("form").forEach(function (form) {
    form.addEventListener("submit", function () {
      var btn = form.querySelector("button[type=submit][data-loading-text]");
      if (!btn || btn.getAttribute("aria-busy") === "true") return;
      btn.setAttribute("aria-busy", "true");
      btn.setAttribute("data-original-text", btn.textContent);
      btn.textContent = btn.getAttribute("data-loading-text");
    });
  });

  // --- Paid course checkout (Razorpay) -------------------------------------
  var purchaseBtn = document.getElementById("purchase-btn");
  if (purchaseBtn) {
    purchaseBtn.addEventListener("click", function () {
      var slug = purchaseBtn.getAttribute("data-slug");
      var minCents = parseInt(purchaseBtn.getAttribute("data-min-cents"), 10) || 0;
      var emailInput = document.getElementById("purchase-email");
      var amountInput = document.getElementById("purchase-amount");
      var promoInput = document.getElementById("purchase-promo-code");
      var errorBox = document.getElementById("purchase-error");

      function showError(msg) {
        if (errorBox) {
          errorBox.textContent = msg;
          errorBox.hidden = false;
        }
      }

      var email = emailInput ? emailInput.value.trim() : "";
      if (emailInput && (!email || email.indexOf("@") === -1)) {
        showError("Please enter a valid email.");
        return;
      }
      var amount = Math.round(parseFloat(amountInput ? amountInput.value : "0") * 100);
      if (isNaN(amount) || amount < minCents) amount = minCents;
      var defaultLabel = amount > 0 ? "Continue" : "Get Free Access";

      if (errorBox) errorBox.hidden = true;
      purchaseBtn.disabled = true;
      purchaseBtn.textContent = amount > 0 ? "Starting checkout…" : "Getting access…";

      var form = new FormData();
      form.append("email", email);
      form.append("amount_cents", String(amount));
      form.append("promo_code", promoInput ? promoInput.value.trim() : "");

      fetch("/courses/" + slug + "/checkout/order", { method: "POST", body: form })
        .then(function (res) {
          return res.json().then(function (data) {
            if (!res.ok) throw new Error(data.error || "Could not start checkout.");
            return data;
          });
        })
        .then(function (order) {
          if (order.free) {
            window.location.href = order.redirect;
            return;
          }
          var rzp = new Razorpay({
            key: order.key_id,
            amount: order.amount,
            currency: order.currency,
            name: order.course_name,
            description: "Course access",
            order_id: order.order_id,
            prefill: { email: order.email },
            handler: function (response) {
              fetch("/courses/" + slug + "/checkout/verify", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({
                  razorpay_order_id: response.razorpay_order_id,
                  razorpay_payment_id: response.razorpay_payment_id,
                  razorpay_signature: response.razorpay_signature,
                }),
              })
                .then(function (res) {
                  return res.json();
                })
                .then(function (result) {
                  if (result.redirect) {
                    window.location.href = result.redirect;
                  } else {
                    showError(result.error || "Payment could not be verified.");
                  }
                })
                .catch(function () {
                  showError("Payment could not be verified. Contact support with your payment ID.");
                });
            },
            modal: {
              ondismiss: function () {
                purchaseBtn.disabled = false;
                purchaseBtn.textContent = defaultLabel;
              },
            },
          });
          rzp.open();
          purchaseBtn.disabled = false;
          purchaseBtn.textContent = defaultLabel;
        })
        .catch(function (err) {
          showError(err.message || "Could not start checkout.");
          purchaseBtn.disabled = false;
          purchaseBtn.textContent = defaultLabel;
        });
    });
  }

  // --- Architecture Review: fixed-price checkout, no calendar slot -------
  var auditBtn = document.getElementById("audit-btn");
  if (auditBtn) {
    auditBtn.addEventListener("click", function () {
      var nameInput = document.getElementById("audit-name");
      var emailInput = document.getElementById("audit-email");
      var companyInput = document.getElementById("audit-company");
      var briefInput = document.getElementById("audit-brief");
      var errorBox = document.getElementById("audit-error");

      function showAuditError(msg) {
        if (errorBox) {
          errorBox.textContent = msg;
          errorBox.hidden = false;
        }
      }

      var name = nameInput ? nameInput.value.trim() : "";
      var email = emailInput ? emailInput.value.trim() : "";
      if (!name) {
        showAuditError("Please enter your name.");
        return;
      }
      if (!email || email.indexOf("@") === -1) {
        showAuditError("Please enter a valid email.");
        return;
      }

      if (errorBox) errorBox.hidden = true;
      var defaultAuditLabel = auditBtn.textContent;
      auditBtn.disabled = true;
      auditBtn.textContent = "Starting checkout…";

      var form = new FormData();
      form.append("name", name);
      form.append("email", email);
      form.append("company", companyInput ? companyInput.value.trim() : "");
      form.append("brief", briefInput ? briefInput.value.trim() : "");

      fetch("/consultancy/audit/checkout/order", { method: "POST", body: form })
        .then(function (res) {
          return res.json().then(function (data) {
            if (!res.ok) throw new Error(data.error || "Could not start checkout.");
            return data;
          });
        })
        .then(function (order) {
          var rzp = new Razorpay({
            key: order.key_id,
            amount: order.amount,
            currency: order.currency,
            name: "Architecture Review",
            description: order.name,
            order_id: order.order_id,
            prefill: { email: order.email, name: order.name },
            handler: function (response) {
              fetch("/consultancy/audit/checkout/verify", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({
                  razorpay_order_id: response.razorpay_order_id,
                  razorpay_payment_id: response.razorpay_payment_id,
                  razorpay_signature: response.razorpay_signature,
                }),
              })
                .then(function (res) { return res.json(); })
                .then(function (result) {
                  if (result.redirect) {
                    window.location.href = result.redirect;
                  } else {
                    showAuditError(result.error || "Payment could not be verified.");
                  }
                })
                .catch(function () {
                  showAuditError("Payment could not be verified. Contact support with your payment ID.");
                });
            },
            modal: {
              ondismiss: function () {
                auditBtn.disabled = false;
                auditBtn.textContent = defaultAuditLabel;
              },
            },
          });
          rzp.open();
          auditBtn.disabled = false;
          auditBtn.textContent = defaultAuditLabel;
        })
        .catch(function (err) {
          showAuditError(err.message || "Could not start checkout.");
          auditBtn.disabled = false;
          auditBtn.textContent = defaultAuditLabel;
        });
    });
  }

  // --- Consultancy booking: pick a slot, pay via Razorpay ----------------
  var bookingContact = document.getElementById("booking-contact");
  var selectedLabel = document.getElementById("selected-slot-label");
  var payBtn = document.getElementById("booking-pay-btn");
  var selectedSlot = "";
  if (bookingContact && payBtn) {
    document.querySelectorAll(".slot-btn").forEach(function (btn) {
      btn.addEventListener("click", function () {
        if (btn.disabled) return;
        document.querySelectorAll(".slot-btn.selected").forEach(function (b) {
          b.classList.remove("selected");
        });
        btn.classList.add("selected");
        selectedSlot = btn.getAttribute("data-slot");
        if (selectedLabel) selectedLabel.textContent = btn.getAttribute("data-label");
        bookingContact.hidden = false;
        bookingContact.scrollIntoView({ behavior: "smooth", block: "center" });
      });
    });

    payBtn.addEventListener("click", function () {
      var errorBox = document.getElementById("booking-error");
      var messageInput = document.getElementById("booking-message");
      var promoInput = document.getElementById("booking-promo-code");
      function showError(msg) {
        if (errorBox) {
          errorBox.textContent = msg;
          errorBox.hidden = false;
        }
      }
      if (!selectedSlot) {
        showError("Please pick a time slot first.");
        return;
      }
      if (errorBox) errorBox.hidden = true;
      payBtn.disabled = true;
      var originalText = payBtn.textContent;
      payBtn.textContent = "Starting checkout…";

      var form = new FormData();
      form.append("slot", selectedSlot);
      form.append("message", messageInput ? messageInput.value : "");
      form.append("promo_code", promoInput ? promoInput.value.trim() : "");

      fetch("/booking/checkout/order", { method: "POST", body: form })
        .then(function (res) {
          return res.json().then(function (data) {
            if (!res.ok) throw new Error(data.error || "Could not start checkout.");
            return data;
          });
        })
        .then(function (order) {
          var rzp = new Razorpay({
            key: order.key_id,
            amount: order.amount,
            currency: order.currency,
            name: "Consultancy session",
            description: selectedLabel ? selectedLabel.textContent : "1-hour session",
            order_id: order.order_id,
            prefill: { email: order.email, name: order.name },
            handler: function (response) {
              fetch("/booking/checkout/verify", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({
                  razorpay_order_id: response.razorpay_order_id,
                  razorpay_payment_id: response.razorpay_payment_id,
                  razorpay_signature: response.razorpay_signature,
                }),
              })
                .then(function (res) { return res.json(); })
                .then(function (result) {
                  if (result.redirect) {
                    window.location.href = result.redirect;
                  } else {
                    showError(result.error || "Payment could not be verified.");
                  }
                })
                .catch(function () {
                  showError("Payment could not be verified. Contact support with your payment ID.");
                });
            },
            modal: {
              ondismiss: function () {
                payBtn.disabled = false;
                payBtn.textContent = originalText;
              },
            },
          });
          rzp.open();
          payBtn.disabled = false;
          payBtn.textContent = originalText;
        })
        .catch(function (err) {
          showError(err.message || "Could not start checkout.");
          payBtn.disabled = false;
          payBtn.textContent = originalText;
        });
    });
  }
})();
