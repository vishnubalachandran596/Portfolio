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

  // --- Fixed-price software checkout (Razorpay) ---------------------------
  var softwarePurchaseBtn = document.getElementById("software-purchase-btn");
  if (softwarePurchaseBtn) {
    softwarePurchaseBtn.addEventListener("click", function () {
      var slug = softwarePurchaseBtn.getAttribute("data-slug");
      var promoInput = document.getElementById("software-promo-code");
      var errorBox = document.getElementById("purchase-error");
      var defaultLabel = softwarePurchaseBtn.textContent;

      function showError(msg) {
        if (errorBox) {
          errorBox.textContent = msg;
          errorBox.hidden = false;
        }
      }

      if (errorBox) errorBox.hidden = true;
      softwarePurchaseBtn.disabled = true;
      softwarePurchaseBtn.textContent = "Starting checkout…";

      var form = new FormData();
      form.append("promo_code", promoInput ? promoInput.value.trim() : "");

      fetch("/software/" + slug + "/checkout/order", { method: "POST", body: form })
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
            name: order.software_name,
            description: "Software purchase",
            order_id: order.order_id,
            prefill: { email: order.email },
            handler: function (response) {
              fetch("/software/" + slug + "/checkout/verify", {
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
                softwarePurchaseBtn.disabled = false;
                softwarePurchaseBtn.textContent = defaultLabel;
              },
            },
          });
          rzp.open();
          softwarePurchaseBtn.disabled = false;
          softwarePurchaseBtn.textContent = defaultLabel;
        })
        .catch(function (err) {
          showError(err.message || "Could not start checkout.");
          softwarePurchaseBtn.disabled = false;
          softwarePurchaseBtn.textContent = defaultLabel;
        });
    });
  }

  // --- Optional article contribution (Razorpay) --------------------------
  var contributeBtn = document.getElementById("contribute-btn");
  if (contributeBtn) {
    contributeBtn.addEventListener("click", function () {
      var slug = contributeBtn.getAttribute("data-slug");
      var minCents = parseInt(contributeBtn.getAttribute("data-min-cents"), 10) || 100;
      var emailInput = document.getElementById("contribute-email");
      var amountInput = document.getElementById("contribute-amount");
      var errorBox = document.getElementById("contribute-error");

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
      if (isNaN(amount) || amount < minCents) {
        showError("Enter an amount of at least " + (minCents / 100) + ".");
        return;
      }

      if (errorBox) errorBox.hidden = true;
      contributeBtn.disabled = true;
      contributeBtn.textContent = "Starting checkout…";

      var form = new FormData();
      form.append("email", email);
      form.append("amount_cents", String(amount));

      fetch("/articles/" + slug + "/checkout/order", { method: "POST", body: form })
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
            name: order.article_title,
            description: "Article contribution",
            order_id: order.order_id,
            prefill: { email: order.email },
            handler: function (response) {
              fetch("/articles/" + slug + "/checkout/verify", {
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
                    contributeBtn.disabled = false;
                    contributeBtn.textContent = "Contribute";
                  }
                })
                .catch(function () {
                  showError("Payment could not be verified. Contact support with your payment ID.");
                  contributeBtn.disabled = false;
                  contributeBtn.textContent = "Contribute";
                });
            },
            modal: {
              ondismiss: function () {
                contributeBtn.disabled = false;
                contributeBtn.textContent = "Contribute";
              },
            },
          });
          rzp.open();
          contributeBtn.disabled = false;
          contributeBtn.textContent = "Contribute";
        })
        .catch(function (err) {
          showError(err.message || "Could not start checkout.");
          contributeBtn.disabled = false;
          contributeBtn.textContent = "Contribute";
        });
    });
  }

  // --- Consultancy booking: pick a slot, confirm — free, no payment step -
  var bookingContact = document.getElementById("booking-contact");
  var selectedLabel = document.getElementById("selected-slot-label");
  var confirmBtn = document.getElementById("booking-confirm-btn");
  var selectedSlot = "";
  if (bookingContact && confirmBtn) {
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

    confirmBtn.addEventListener("click", function () {
      var errorBox = document.getElementById("booking-error");
      var messageInput = document.getElementById("booking-message");
      var phoneInput = document.getElementById("booking-phone");
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
      var phone = phoneInput ? phoneInput.value.trim() : "";
      if (!phone) {
        showError("Please enter a contact number.");
        return;
      }
      if (errorBox) errorBox.hidden = true;
      confirmBtn.disabled = true;
      var originalText = confirmBtn.textContent;
      confirmBtn.textContent = "Confirming…";

      var form = new FormData();
      form.append("slot", selectedSlot);
      form.append("phone", phone);
      form.append("message", messageInput ? messageInput.value : "");

      fetch("/booking/confirm", { method: "POST", body: form })
        .then(function (res) {
          return res.json().then(function (data) {
            if (!res.ok) throw new Error(data.error || "Could not confirm the booking.");
            return data;
          });
        })
        .then(function (result) {
          window.location.href = result.redirect;
        })
        .catch(function (err) {
          showError(err.message || "Could not confirm the booking.");
          confirmBtn.disabled = false;
          confirmBtn.textContent = originalText;
        });
    });
  }

  // --- Skills grid: click/tap to pin a card's summary and tags open ----
  // The achievement summary is not on the face. Hover and keyboard focus
  // reveal it with the full tag list in CSS alone, as a transient overlay
  // that does not reflow the grid. This adds the click/tap toggle on top:
  // a click pins the card open, a second click unpins it, and on a
  // touchscreen — where there is no hover at all — that toggle is the only
  // reveal of the summary. It is also what keeps `aria-expanded` truthful,
  // on both of a card's controls (the face and "and N more"), since they
  // both point at the same panel. Nothing here resizes a card or scrolls
  // the page: the revealed panel is positioned over its own card, so the
  // grid can't reflow (see .skill-card-detail).
  var skillCards = document.querySelectorAll("[data-skill-card]");
  var skillsGrid = document.getElementById("skills-grid");
  var expandAllBtn = document.querySelector("[data-skills-expand-all]");
  if (skillCards.length) {
    // "Expand all" is a whole display mode rather than nine per-card
    // toggles fired in a loop — the per-card panel is absolutely
    // positioned over its own card, which is what stops the grid
    // reflowing, and nine of those at once would overlap each other. The
    // modifier class on the grid swaps to a layout where the panels are
    // back in normal flow. See the .skills-grid block in style.css.
    var isExpandedAll = function () {
      return !!skillsGrid && skillsGrid.classList.contains("is-expanded-all");
    };

    var skillControls = function (card) {
      return card.querySelectorAll(".skill-card-toggle, [data-skill-more]");
    };
    var setSkillExpanded = function (card, open) {
      var controls = skillControls(card);
      for (var i = 0; i < controls.length; i++) {
        controls[i].setAttribute("aria-expanded", String(open));
      }
    };
    // Unpinning has to survive the pointer still sitting on the card, or
    // :hover re-reveals the panel and the second click reads as a no-op.
    // .is-dismissed suppresses the preview for the rest of that gesture;
    // the pointer or the keyboard arriving at the card again clears it.
    var closeSkillCard = function (card, dismiss) {
      card.classList.remove("is-open");
      card.classList.toggle("is-dismissed", !!dismiss);
      setSkillExpanded(card, false);
    };
    var openSkillCard = function (card) {
      card.classList.add("is-open");
      card.classList.remove("is-dismissed");
      setSkillExpanded(card, true);
    };
    // One card pinned at a time: a second open panel is never wrong on
    // screen (each is clipped to its own card) but it makes "which one am
    // I looking at" ambiguous for no gain.
    var closeSkillCards = function (except) {
      skillCards.forEach(function (card) {
        if (card !== except) closeSkillCard(card, false);
      });
    };

    skillCards.forEach(function (card) {
      var more = card.querySelector("[data-skill-more]");
      // Same progressive-enhancement shape as the Expand all button below:
      // the control cannot do anything without JS, so it ships hidden in
      // the markup and is revealed here. With JS off the card still has
      // category, level, and four chips on the face, and the CSS
      // hover/focus overlay still reveals the summary plus the full list.
      if (more) more.hidden = false;

      // One listener on the card rather than one per control. The face and
      // "and N more" mean the same thing, a click on the chips between
      // them should mean it too, and Enter/Space on either button arrives
      // here as a click — so a single handler keeps one source of truth
      // for the state instead of two that can disagree.
      card.addEventListener("click", function () {
        // In expanded mode every panel is already shown, so a per-card
        // toggle has nothing to say and pretending otherwise would just
        // desync aria-expanded from what is on screen.
        if (isExpandedAll()) return;
        if (card.classList.contains("is-open")) {
          closeSkillCard(card, true);
        } else {
          closeSkillCards(card);
          openSkillCard(card);
        }
      });
      // A pointer arriving is a new hover gesture, so whatever the last one
      // dismissed is forgotten. Clearing on the way in rather than on the
      // way out matters: on the way out the card is still hidden either
      // way, on the way in the difference is whether it previews at all.
      card.addEventListener("mouseenter", function () {
        card.classList.remove("is-dismissed");
      });
      // Tabbing into a card is as deliberate as clicking one: it unpins
      // whichever other card was pinned, and clears this one's dismissal so
      // the keyboard preview isn't suppressed by a gesture that's over.
      card.addEventListener("focusin", function () {
        card.classList.remove("is-dismissed");
        if (!isExpandedAll()) closeSkillCards(card);
      });
    });
    document.addEventListener("click", function (e) {
      if (isExpandedAll()) return;
      if (!e.target.closest("[data-skill-card]")) closeSkillCards(null);
    });
    document.addEventListener("keydown", function (e) {
      if (e.key !== "Escape") return;
      // Escape is the way out of either state.
      if (isExpandedAll() && expandAllBtn) {
        expandAllBtn.click();
        return;
      }
      // Escape closes whatever is on screen, which for a keyboard visitor
      // is the focus preview as often as it is a pinned card — hence the
      // dismissal, which is the part that suppresses a preview.
      skillCards.forEach(function (card) {
        closeSkillCard(card, true);
      });
    });

    if (skillsGrid && expandAllBtn) {
      var expandLabel = expandAllBtn.querySelector("[data-skills-expand-label]");
      var expandStatus = document.querySelector("[data-skills-status]");
      // The button does nothing without JS, so it ships hidden and is
      // revealed here — same progressive-enhancement shape as the other
      // JS-only controls in this file.
      expandAllBtn.hidden = false;
      expandAllBtn.addEventListener("click", function () {
        var expand = !isExpandedAll();
        skillsGrid.classList.toggle("is-expanded-all", expand);
        expandAllBtn.setAttribute("aria-expanded", String(expand));
        if (expandLabel) expandLabel.textContent = expand ? "Collapse all" : "Expand all";
        // Entering the mode clears any single pinned card so the two
        // reveal mechanisms can't both claim to be in charge; leaving it
        // returns every card to the resting state — four chips, an "and N
        // more" control, no summary on the face, and nothing stuck open.
        closeSkillCards(null);
        skillCards.forEach(function (card) {
          setSkillExpanded(card, expand);
        });
        // The mode change is otherwise a purely visual event.
        if (expandStatus) {
          expandStatus.textContent = expand
            ? "All skill groups expanded."
            : "All skill groups collapsed.";
        }
      });
    }
  }

  // --- Article list thumbs-up ------------------------------------------
  // Each row's vote button is a sibling of the row's link (not nested
  // inside it), specifically so a click here never falls through to the
  // link's navigation — a button nested inside an <a> previously caused
  // exactly that bug.
  var thumbsButtons = document.querySelectorAll(".thumbs-count[data-slug]");
  for (var ti = 0; ti < thumbsButtons.length; ti++) {
    (function (btn) {
      btn.addEventListener("click", function (e) {
        e.preventDefault();
        e.stopPropagation();
        if (btn.disabled) return;
        btn.disabled = true;
        var slug = btn.dataset.slug;
        fetch("/articles/" + slug + "/thumbs-up", { method: "POST" })
          .then(function (r) {
            if (r.status === 401) {
              window.location.href = "/login?next=" + encodeURIComponent("/articles/" + slug);
              throw new Error("login required");
            }
            return r.json();
          })
          .then(function (data) {
            if (typeof data.thumbs_up_count === "number") {
              var numEl = btn.querySelector(".thumbs-count-num");
              if (numEl) numEl.textContent = data.thumbs_up_count;
            }
          })
          .catch(function () { btn.disabled = false; });
      });
    })(thumbsButtons[ti]);
  }

  // --- Chatbot widget ----------------------------------------------------
  // Floating bubble + panel, present only when the server rendered it
  // (gated on BaseData.ChatbotEnabled -- i.e. NVIDIA_API_KEY is set).
  var chatbotWidget = document.getElementById("chatbot-widget");
  if (chatbotWidget) {
    var chatbotToggle = document.getElementById("chatbot-toggle");
    var chatbotClose = document.getElementById("chatbot-close");
    var chatbotExpand = document.getElementById("chatbot-expand");
    var chatbotPanel = document.getElementById("chatbot-panel");
    var chatbotForm = document.getElementById("chatbot-form");
    var chatbotInput = document.getElementById("chatbot-input");
    var chatbotMessages = document.getElementById("chatbot-messages");
    // In-memory only, per the app's own "no server-side session store"
    // rule (CLAUDE.md §3) -- the server is stateless, so the conversation
    // has to live somewhere between requests, and that's here, not a
    // cookie or sessionStorage. Resets on reload, which is fine: a fresh
    // page load is a reasonable place to start a new conversation -- the
    // one deliberate exception is clicking the expand-to-full-page icon
    // below, which hands this same array to the next page via sessionStorage
    // so the conversation keeps going instead of restarting.
    var chatHistory = [];
    var CHATBOT_MAX_HISTORY_TURNS = 6;
    var CHATBOT_TRANSFER_KEY = "chatbotTransferHistory";

    function openChatbot() {
      chatbotPanel.hidden = false;
      chatbotToggle.setAttribute("aria-expanded", "true");
      chatbotInput.focus();
    }
    function closeChatbot() {
      chatbotPanel.hidden = true;
      chatbotToggle.setAttribute("aria-expanded", "false");
    }
    // chatbotToggle/chatbotClose are absent on the full-page chat
    // (/ai-assistant) -- there the panel is the whole page, already open,
    // with nothing to toggle or close. Same markup/ids otherwise, so
    // everything else below runs unmodified in either shell.
    if (chatbotToggle) {
      chatbotToggle.addEventListener("click", function () {
        if (chatbotPanel.hidden) openChatbot();
        else closeChatbot();
      });
    }
    if (chatbotClose) chatbotClose.addEventListener("click", closeChatbot);
    // Hand the in-progress conversation to /ai-assistant rather than
    // losing it on navigation -- sessionStorage, not a server session,
    // same "no in-process state" reasoning as chatHistory itself above.
    // A plain synchronous write before the browser's normal navigation
    // for this link proceeds; no preventDefault needed.
    if (chatbotExpand) {
      chatbotExpand.addEventListener("click", function () {
        try {
          if (chatHistory.length) {
            sessionStorage.setItem(CHATBOT_TRANSFER_KEY, JSON.stringify(chatHistory));
          }
        } catch (err) {}
      });
    }
    if (!chatbotToggle) chatbotInput.focus();

    function appendMessage(text, who) {
      var el = document.createElement("div");
      el.className = "chatbot-message chatbot-message-" + who;
      el.textContent = text;
      chatbotMessages.appendChild(el);
      chatbotMessages.scrollTop = chatbotMessages.scrollHeight;
      return el;
    }

    function appendSources(sources) {
      if (!sources || !sources.length) return;
      var wrap = document.createElement("div");
      wrap.className = "chatbot-sources";
      for (var si = 0; si < sources.length; si++) {
        var a = document.createElement("a");
        a.href = sources[si].url;
        a.textContent = sources[si].title;
        wrap.appendChild(a);
      }
      chatbotMessages.appendChild(wrap);
      chatbotMessages.scrollTop = chatbotMessages.scrollHeight;
    }

    // Other half of the expand-click handoff above: a transfer left here
    // by the page that sent us here replaces the default greeting with
    // the real conversation so far, and chatHistory picks up from where
    // it left off instead of starting over. Read-once -- removed
    // immediately so a later reload of this same page doesn't replay it.
    (function restoreTransferredHistory() {
      var raw;
      try {
        raw = sessionStorage.getItem(CHATBOT_TRANSFER_KEY);
        sessionStorage.removeItem(CHATBOT_TRANSFER_KEY);
      } catch (err) {
        return;
      }
      if (!raw) return;
      var transferred;
      try {
        transferred = JSON.parse(raw);
      } catch (err) {
        return;
      }
      if (!transferred || !transferred.length) return;
      chatbotMessages.innerHTML = "";
      for (var hi = 0; hi < transferred.length; hi++) {
        var turn = transferred[hi];
        appendMessage(turn.content, turn.role === "user" ? "user" : "bot");
      }
      chatHistory = transferred;
    })();

    chatbotForm.addEventListener("submit", function (e) {
      e.preventDefault();
      var message = chatbotInput.value.trim();
      if (!message) return;
      appendMessage(message, "user");
      chatbotInput.value = "";
      chatbotInput.disabled = true;
      var pending = appendMessage("…", "bot");

      fetch("/chat", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ message: message, history: chatHistory })
      })
        .then(function (r) { return r.json().then(function (data) { return { status: r.status, data: data }; }); })
        .then(function (res) {
          var reply = res.data.error || res.data.reply || "Something went wrong.";
          pending.textContent = reply;
          if (res.status === 200 && res.data.sources) appendSources(res.data.sources);
          // Only a real answer extends the conversation -- an error (rate
          // limit, server problem) isn't something the next question
          // should be "continuing", and the server's own decline reply
          // ("I can only answer questions about...") isn't useful context
          // for a follow-up either.
          if (res.status === 200 && res.data.reply) {
            chatHistory.push({ role: "user", content: message });
            chatHistory.push({ role: "assistant", content: res.data.reply });
            if (chatHistory.length > CHATBOT_MAX_HISTORY_TURNS) {
              chatHistory = chatHistory.slice(chatHistory.length - CHATBOT_MAX_HISTORY_TURNS);
            }
          }
        })
        .catch(function () {
          pending.textContent = "Something went wrong reaching the chatbot. Please try again.";
        })
        .then(function () {
          chatbotInput.disabled = false;
          chatbotInput.focus();
        });
    });
  }
})();
