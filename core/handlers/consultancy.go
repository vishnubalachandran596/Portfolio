package handlers

import (
	"fmt"
	"html/template"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/authutil"
	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
	"github.com/vishnubalachandran596/portfolio/core/scheduler"
)

type ConsultancyData struct {
	BaseData
	AuditPriceDisplay    string
	RetainerRangeDisplay string
	FAQ                  []FAQItem
	// CaseStudies is the same published list About renders — one
	// canonical source of proof-of-work for both pages. Empty until
	// Vishnu adds a real one; see migration 060.
	CaseStudies []models.CaseStudy
	JSONLD      template.JS
}

// consultancyFAQ builds the six question/answer pairs the page's "Questions
// people ask before they get in touch" section shows, parameterized on the
// region-resolved price figures so the cost answer can never disagree with
// the tier cards above it. This is the single source for both the visible
// <details> accordion in consultancy.html and the FAQPage schema
// (consultancyJSONLD in schema.go) — see FAQItem's doc comment.
func consultancyFAQ(auditDisplay, retainerDisplay string) []FAQItem {
	return []FAQItem{
		{
			Question: "How is this different from hiring a full-time architect?",
			Answer:   "A full-time architect is a headcount decision — months of hiring, then a permanent cost whether or not there's an architecture-sized problem that quarter. This is the same judgement, bought by the decision or by the month. An Architecture Review gets you a senior opinion on one specific question in a couple of weeks; a retainer gets you that opinion continuously without the salary, the notice period, or the risk of hiring the wrong person for a role you only half need filled.",
		},
		{
			Question: "What does an architecture review actually deliver?",
			Answer:   "A written findings document, plus a call to walk through it. The document covers the risks I found in what you're building or planning, the trade-offs behind each one, and a specific recommendation — which option to take and why, not a list of things to consider. It's written to be forwarded: something you can put in front of an engineering team or a board without translating it first.",
		},
		{
			Question: "What does a week actually look like on a retainer?",
			Answer:   `Most weeks: a standing review call on whatever rhythm we agree, and async access in between for the questions that can't wait for it — a design doc you want read before it's circulated, a "are we about to regret this?" sanity check, a second opinion on a vendor or a database choice. Quiet weeks are quiet. Busy weeks are the point of having the access.`,
		},
		{
			Question: "How much does a fractional technical advisor cost?",
			Answer:   fmt.Sprintf("A Fractional Advisory retainer here runs %s, and a fixed-scope Architecture Review is typically around %s. Both are priced on outcome rather than hours logged, so there's no timesheet to argue about. A call to work out which one you need — or whether you need either — is free.", retainerDisplay, auditDisplay),
		},
		{
			Question: "How do you handle confidentiality and NDAs?",
			Answer:   "Send your standard mutual NDA with your first message and I'll sign it before we get into specifics. Nothing you share appears anywhere on this site — as a case study or otherwise — without your explicit written agreement. In the other direction: this consulting work sits entirely outside my day job and never touches my employer's clients, codebases, or confidential information.",
		},
		{
			Question: "Do I have to pay anything to talk to you first?",
			Answer:   "No. The free call is the front door for all three options, not a trimmed-down version of them — book a slot and we'll work out what you need. If that turns out to be an Architecture Review or a retainer, we'll scope it from there. If it turns out to be neither, that's a useful answer too.",
		},
	}
}

// ConsultancyPage is the informational page — three tiers, none of them
// touch a payment gateway: the Architecture Review and the Fractional
// Advisory retainer both route to /contact to scope on a call first, and
// the hourly session is a free scheduled call via /booking. No payment
// integration exists anywhere on this site except course purchases
// (checkout.go) — see docs/ARCHITECTURE.md §6.
func (a *App) ConsultancyPage(w http.ResponseWriter, r *http.Request) {
	base := a.base(r, "Technical Consultant — Architecture Reviews & Fractional Advisory | "+a.Cfg.SiteName, "Architecture reviews with written findings and fractional advisory retainers from an 8+ year banking and fintech platform architect. The first call is free.", "consultancy")

	caseStudies, err := a.CaseStudies.ListPublished(r.Context())
	if err != nil {
		a.serverError(w, "load case studies", err)
		return
	}

	auditDisplay := models.AuditPriceDisplay()
	retainerDisplay := models.RetainerRangeDisplay()
	faq := consultancyFAQ(auditDisplay, retainerDisplay)

	a.View.Render(w, http.StatusOK, "consultancy", ConsultancyData{
		BaseData:             base,
		AuditPriceDisplay:    auditDisplay,
		RetainerRangeDisplay: retainerDisplay,
		FAQ:                  faq,
		CaseStudies:          caseStudies,
		JSONLD:               consultancyJSONLD(base.CanonicalURL, base.SiteName, base.MetaDescription, retainerDisplay, faq),
	})
}

type BookingData struct {
	BaseData
	Days      []scheduler.DaySlots
	Error     string
	UserEmail string
	UserName  string
}

// Throttle limits for public booking. Every attempt that reaches the insert
// counts (success or not), so a bot cannot fill the calendar: the lock
// starts at the limit and then doubles up to an hour.
const (
	throttleBooking       = "booking"
	bookingMaxPerIP       = 6
	bookingMaxPerEmail    = 3
	bookingMaxFieldLength = 1000
)

// Throttle limits for the public contact form, same mechanism as booking.
const (
	throttleContact         = "contact"
	contactMaxPerIP         = 5
	contactMaxPerEmail      = 3
	contactMaxMessageLength = 5000
)

// clipRunes shortens s to at most n runes.
func clipRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// BookingPage is open to everyone: no login is needed to book. The hourly
// session is a free scheduled call — no price, no payment step. A signed-in
// visitor just gets their name and email pre-filled.
func (a *App) BookingPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	days, err := a.availableDays(r)
	if err != nil {
		a.serverError(w, "load availability", err)
		return
	}

	base := a.base(r, "Book a Free Consultancy Call — "+a.Cfg.SiteName, "Book a free 1:1 consultancy call. No account needed: pick an hourly slot in the next 30 days.", "consultancy")

	userEmail, userNameVal := "", ""
	if uid, ok := a.Auth.UserID(r); ok {
		if u, err := a.Users.GetByID(ctx, uid); err == nil {
			userEmail = u.Email
			userNameVal = u.Name
		}
	}

	a.View.Render(w, http.StatusOK, "booking", BookingData{
		BaseData:  base,
		Days:      days,
		Error:     r.URL.Query().Get("error"),
		UserEmail: userEmail,
		UserName:  userNameVal,
	})
}

func bookingJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// BookingConfirm reserves the chosen slot for anyone (no login). The slot is
// claimed by the database — a unique index on the live slot start plus a
// no-overlap exclusion constraint — so two simultaneous requests cannot both
// win; the loser sees "just booked". Every booking, including a free one,
// queues a confirmation. The response still carries the booking code and the
// private manage token, which is shown once and kept only as a hash.
func (a *App) BookingConfirm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// This form is submitted via fetch() with a FormData body (multipart), so
	// ParseMultipartForm is required — plain ParseForm never reads it.
	if err := r.ParseMultipartForm(1 << 20); err != nil && err != http.ErrNotMultipart {
		bookingJSONError(w, http.StatusBadRequest, "Invalid form submission.")
		return
	}
	// Honeypot: real visitors never fill this hidden field. Answer as if it
	// worked so a bot learns nothing.
	if r.FormValue("website") != "" {
		writeJSON(w, http.StatusOK, map[string]any{"redirect": "/booking"})
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	phone := strings.TrimSpace(r.FormValue("phone"))
	message := strings.TrimSpace(r.FormValue("message"))
	if spammyName(name) {
		writeJSON(w, http.StatusOK, map[string]any{"redirect": "/booking"}) // bot: answer as if it worked
		return
	}
	if !validPersonName(name) {
		bookingJSONError(w, http.StatusBadRequest, "Please enter your real name: first name, or first and last name (English letters only).")
		return
	}
	if !englishText(message) || !validPhone(phone) {
		bookingJSONError(w, http.StatusBadRequest, "Please use English for the topic, and a valid phone number or leave it empty.")
		return
	}
	if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email || len(email) > 254 || !asciiOnly(email) {
		bookingJSONError(w, http.StatusBadRequest, "Please enter a valid email address.")
		return
	}
	if len(phone) > 30 || len([]rune(message)) > bookingMaxFieldLength {
		bookingJSONError(w, http.StatusBadRequest, "Your phone number or message is too long.")
		return
	}
	slotStart, err := time.Parse(time.RFC3339, strings.TrimSpace(r.FormValue("slot")))
	if err != nil {
		bookingJSONError(w, http.StatusBadRequest, "That time slot is no longer valid, please pick another.")
		return
	}

	ik := authutil.HashKey(authutil.ClientIP(r))
	ek := authutil.HashKey(email)
	if until, locked := a.throttled(r, throttleBooking, ik, ek); locked {
		bookingJSONError(w, http.StatusTooManyRequests, lockedMessage(until))
		return
	}

	rules, err := a.Availability.ListActive(ctx)
	if err != nil {
		a.serverError(w, "load availability", err)
		return
	}
	msg, slotMinutes := a.bookableSlotError(rules, slotStart)
	if msg != "" {
		bookingJSONError(w, http.StatusBadRequest, msg)
		return
	}
	if n, err := a.Bookings.CountActiveByEmail(ctx, email); err != nil {
		a.serverError(w, "count bookings", err)
		return
	} else if n >= models.BookingMaxActivePerEmail {
		bookingJSONError(w, http.StatusConflict, fmt.Sprintf("That email already has %d upcoming bookings. Cancel or reschedule one from its manage link first.", n))
		return
	}

	token, err := authutil.RandomHex(24)
	if err != nil {
		a.serverError(w, "generate manage token", err)
		return
	}
	booking := models.Booking{
		Name:      name,
		Email:     email,
		Phone:     phone,
		Message:   message,
		SlotStart: slotStart,
		SlotEnd:   slotStart.Add(time.Duration(slotMinutes) * time.Minute),
	}
	if uid, ok := a.Auth.UserID(r); ok {
		booking.UserID = uid // links the booking to /account for signed-in users
	}

	a.recordFailure(r, throttleBooking, bookingMaxPerIP, ik)
	a.recordFailure(r, throttleBooking, bookingMaxPerEmail, ek)
	if !a.humanVerified(r) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "We could not confirm you are human. Please complete the check and try again."})
		return
	}

	id, code, err := a.Bookings.CreatePublic(ctx, booking, authutil.HashKey(token))
	if err != nil {
		if err == repo.ErrSlotTaken {
			bookingJSONError(w, http.StatusConflict, "That time was just booked by someone else, please pick another.")
			return
		}
		a.serverError(w, "reserve booking", err)
		return
	}

	phoneLine := ""
	if phone != "" {
		phoneLine = "\nPhone: " + phone
	}
	a.Telegram.Send(fmt.Sprintf("📅 Consultancy call booked (%s)\nName: %s\nEmail: %s%s\nWhen: %s",
		code, name, email, phoneLine, slotStart.In(a.Timezone).Format("Mon, Jan 2 · 3:04 PM MST")))
	booking.ID = id
	booking.Code = code
	a.emailBooking(ctx, models.EmailBookingNew, booking, slotStart, booking.SlotEnd, token)

	writeJSON(w, http.StatusOK, map[string]any{
		"code":        code,
		"when":        slotStart.In(a.Timezone).Format("Mon, Jan 2 2006 · 3:04 PM MST"),
		"manage_path": "/booking/manage/" + token,
	})
}

type ContactData struct {
	BaseData
	Profile   models.Profile
	Success   bool
	Error     bool
	ErrorText string
}

func (a *App) ContactPage(w http.ResponseWriter, r *http.Request) {
	profile, err := a.Profile.Get(r.Context())
	if err != nil {
		a.serverError(w, "load profile", err)
		return
	}
	a.View.Render(w, http.StatusOK, "contact", ContactData{
		BaseData:  a.base(r, "Contact — "+a.Cfg.SiteName, "Get in touch, or book a consultancy session directly.", "contact"),
		Profile:   profile,
		Success:   r.URL.Query().Get("sent") == "1",
		Error:     r.URL.Query().Get("error") != "",
		ErrorText: contactErrorText(r.URL.Query().Get("error")),
	})
}

// contactErrorText is the banner for each /contact?error= code.
func contactErrorText(code string) string {
	switch code {
	case "name":
		return "Please enter your real name: a first name, or first and last name (English letters only)."
	case "english":
		return "Please write your message in English (Latin letters)."
	case "phone":
		return "Please enter a valid phone number, or leave it empty."
	case "human":
		return "We could not confirm you are human. Please complete the check below and send again."
	}
	return "Please fill in your name, a valid email, and a message."
}

// ContactSubmit records a /contact form submission (name, email, phone,
// message — phone optional) and notifies the admin over Telegram.
// The consultancy page's "Discuss a review" and "Discuss a retainer"
// links land here. A confirmation is queued to the address they
// submitted. No login required — this is the one way a visitor who isn't
// ready to book a call yet can still reach out. A queue problem does not
// undo the saved message.
func (a *App) ContactSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/contact?error=1", http.StatusSeeOther)
		return
	}
	// Honeypot: real visitors never fill this hidden field. Answer as if it
	// worked so a bot learns nothing.
	if r.FormValue("website") != "" {
		http.Redirect(w, r, "/contact?sent=1", http.StatusSeeOther)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	phone := strings.TrimSpace(r.FormValue("phone"))
	message := strings.TrimSpace(r.FormValue("message"))
	// A name that carries a link or an address is a bot: drop it quietly (not
	// stored, not emailed, no alert) and answer as if it worked.
	if spammyName(name) {
		http.Redirect(w, r, "/contact?sent=1", http.StatusSeeOther)
		return
	}
	switch {
	case !validPersonName(name):
		http.Redirect(w, r, "/contact?error=name", http.StatusSeeOther)
		return
	case !validPhone(phone):
		http.Redirect(w, r, "/contact?error=phone", http.StatusSeeOther)
		return
	case message == "" || len([]rune(message)) > contactMaxMessageLength:
		http.Redirect(w, r, "/contact?error=1", http.StatusSeeOther)
		return
	case !englishText(message):
		http.Redirect(w, r, "/contact?error=english", http.StatusSeeOther)
		return
	}
	if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email || len(email) > 254 || !asciiOnly(email) {
		http.Redirect(w, r, "/contact?error=1", http.StatusSeeOther)
		return
	}
	// Per-IP and per-address limit, counted in Postgres so it holds across
	// serverless instances. Every submission that gets this far counts.
	ik := authutil.HashKey(authutil.ClientIP(r))
	ek := authutil.HashKey(email)
	if _, locked := a.throttled(r, throttleContact, ik, ek); locked {
		http.Redirect(w, r, "/contact?error=1", http.StatusSeeOther)
		return
	}
	a.recordFailure(r, throttleContact, contactMaxPerIP, ik)
	a.recordFailure(r, throttleContact, contactMaxPerEmail, ek)
	if !a.humanVerified(r) {
		http.Redirect(w, r, "/contact?error=human", http.StatusSeeOther)
		return
	}

	id, err := a.ContactMessages.Create(r.Context(), models.ContactMessage{
		Name: name, Email: email, Phone: phone, Message: message,
	})
	if err != nil {
		a.serverError(w, "record contact message", err)
		return
	}
	a.emailConnectRequest(r.Context(), models.ContactMessage{ID: id, Name: name, Email: email})

	phoneLine := ""
	if phone != "" {
		phoneLine = "\nPhone: " + phone
	}
	a.Telegram.Send(fmt.Sprintf("✉️ Contact form message\nName: %s\nEmail: %s%s\n%s", name, email, phoneLine, clipRunes(message, 1500)))

	http.Redirect(w, r, "/contact?sent=1", http.StatusSeeOther)
}
