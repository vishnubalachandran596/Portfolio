(function () {
  "use strict";

  // --- Mobile nav toggle ---------------------------------------------
  var navToggle = document.getElementById("nav-toggle");
  var nav = document.getElementById("primary-nav");
  if (navToggle && nav) {
    navToggle.addEventListener("click", function () {
      var open = nav.classList.toggle("open");
      navToggle.setAttribute("aria-expanded", open ? "true" : "false");
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
      var errorBox = document.getElementById("purchase-error");

      function showError(msg) {
        if (errorBox) {
          errorBox.textContent = msg;
          errorBox.hidden = false;
        }
      }

      var email = emailInput ? emailInput.value.trim() : "";
      if (!email || email.indexOf("@") === -1) {
        showError("Please enter a valid email.");
        return;
      }
      var amount = Math.round(parseFloat(amountInput ? amountInput.value : "0") * 100);
      if (isNaN(amount) || amount < minCents) amount = minCents;

      if (errorBox) errorBox.hidden = true;
      purchaseBtn.disabled = true;
      purchaseBtn.textContent = "Starting checkout…";

      var form = new FormData();
      form.append("email", email);
      form.append("amount_cents", String(amount));

      fetch("/courses/" + slug + "/checkout/order", { method: "POST", body: form })
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
                purchaseBtn.textContent = "Pay & Unlock";
              },
            },
          });
          rzp.open();
          purchaseBtn.disabled = false;
          purchaseBtn.textContent = "Pay & Unlock";
        })
        .catch(function (err) {
          showError(err.message || "Could not start checkout.");
          purchaseBtn.disabled = false;
          purchaseBtn.textContent = "Pay & Unlock";
        });
    });
  }

  // --- Booking slot picker ------------------------------------------------
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
