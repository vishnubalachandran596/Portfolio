package handlers

import (
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

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
	Success   bool
	Error     string
	UserEmail string
	UserName  string
}

// BookingPage requires a logged-in visitor (see RequireUser in routes.go).
// The hourly session is a free scheduled call — no price, no payment step.
func (a *App) BookingPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	days, err := a.availableDays(r)
	if err != nil {
		a.serverError(w, "load availability", err)
		return
	}

	base := a.base(r, "Book Consultancy — "+a.Cfg.SiteName, "Book a free 1:1 consultancy call.", "consultancy")

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
		Success:   r.URL.Query().Get("booked") == "1",
		Error:     r.URL.Query().Get("error"),
		UserEmail: userEmail,
		UserName:  userNameVal,
	})
}

// BookingConfirm reserves the chosen slot and confirms it immediately —
// no payment gateway involved at all. Still goes through
// CreatePending -> MarkPaid (the same pair a paid booking would have
// used) rather than a new code path, so the slot-race protection (the
// unique constraint on slot_start) applies exactly the same way; a
// synthetic, unique razorpay_order_id ("free-"+token) is only there to
// give MarkPaid something unambiguous to scope its update to — nothing
// ever gets checked against Razorpay.
func (a *App) BookingConfirm(w http.ResponseWriter, r *http.Request) {
	uid, ok := a.Auth.UserID(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Please log in to book a session."})
		return
	}
	ctx := r.Context()
	user, err := a.Users.GetByID(ctx, uid)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Please log in to book a session."})
		return
	}

	// See the matching comment in checkout.go's CheckoutCreateOrder: this
	// form is submitted via fetch() with a FormData body (multipart), so
	// ParseMultipartForm is required — plain ParseForm never reads it.
	if err := r.ParseMultipartForm(1 << 20); err != nil && err != http.ErrNotMultipart {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid form submission."})
		return
	}
	phone := strings.TrimSpace(r.FormValue("phone"))
	if phone == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Please enter a contact number."})
		return
	}
	message := strings.TrimSpace(r.FormValue("message"))
	slotRaw := strings.TrimSpace(r.FormValue("slot"))
	slotStart, err := time.Parse(time.RFC3339, slotRaw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "That time slot is no longer valid, please pick another."})
		return
	}

	rules, err := a.Availability.ListActive(ctx)
	if err != nil {
		a.serverError(w, "load availability", err)
		return
	}
	if !a.slotIsOffered(rules, slotStart) || slotStart.Before(time.Now()) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "That time slot is not available, please pick another."})
		return
	}
	slotMinutes := models.ConsultancyDurationMinutes
	for _, rr := range rules {
		if rr.Weekday == slotStart.In(a.Timezone).Weekday() {
			slotMinutes = rr.SlotMinutes
			break
		}
	}

	token, terr := generateAccessToken()
	if terr != nil {
		a.serverError(w, "generate booking token", terr)
		return
	}
	orderID := "free-" + token

	booking := models.Booking{
		UserID:          uid,
		Name:            user.Name,
		Email:           user.Email,
		Phone:           phone,
		Message:         message,
		SlotStart:       slotStart,
		SlotEnd:         slotStart.Add(time.Duration(slotMinutes) * time.Minute),
		RazorpayOrderID: orderID,
	}
	if _, err := a.Bookings.CreatePending(ctx, booking); err != nil {
		if err == repo.ErrSlotTaken {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "That time was just booked by someone else, please pick another."})
			return
		}
		a.serverError(w, "reserve booking", err)
		return
	}
	if err := a.Bookings.MarkPaid(ctx, orderID, ""); err != nil {
		a.serverError(w, "confirm booking", err)
		return
	}

	phoneLine := ""
	if phone != "" {
		phoneLine = "\nPhone: " + phone
	}
	a.Telegram.Send(fmt.Sprintf("📅 Consultancy call booked\nName: %s\nEmail: %s%s\nWhen: %s",
		user.Name, user.Email, phoneLine, slotStart.In(a.Timezone).Format("Mon, Jan 2 · 3:04 PM MST")))

	writeJSON(w, http.StatusOK, map[string]any{"redirect": "/booking?booked=1"})
}

type ContactData struct {
	BaseData
	Profile models.Profile
	Success bool
	Error   bool
}

func (a *App) ContactPage(w http.ResponseWriter, r *http.Request) {
	profile, err := a.Profile.Get(r.Context())
	if err != nil {
		a.serverError(w, "load profile", err)
		return
	}
	a.View.Render(w, http.StatusOK, "contact", ContactData{
		BaseData: a.base(r, "Contact — "+a.Cfg.SiteName, "Get in touch, or book a consultancy session directly.", "contact"),
		Profile:  profile,
		Success:  r.URL.Query().Get("sent") == "1",
		Error:    r.URL.Query().Get("error") == "1",
	})
}

// ContactSubmit records a /contact form submission (name, email, phone,
// message — phone optional) and notifies the admin over Telegram, the
// same notification mechanism every other lead/event on this site uses.
// No login required — this is the one way a visitor who isn't ready to
// book a call yet can still reach out.
func (a *App) ContactSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/contact?error=1", http.StatusSeeOther)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	email := strings.TrimSpace(r.FormValue("email"))
	phone := strings.TrimSpace(r.FormValue("phone"))
	message := strings.TrimSpace(r.FormValue("message"))
	if name == "" || email == "" || !strings.Contains(email, "@") || message == "" {
		http.Redirect(w, r, "/contact?error=1", http.StatusSeeOther)
		return
	}

	if _, err := a.ContactMessages.Create(r.Context(), models.ContactMessage{
		Name: name, Email: email, Phone: phone, Message: message,
	}); err != nil {
		a.serverError(w, "record contact message", err)
		return
	}

	phoneLine := ""
	if phone != "" {
		phoneLine = "\nPhone: " + phone
	}
	a.Telegram.Send(fmt.Sprintf("✉️ Contact form message\nName: %s\nEmail: %s%s\n%s", name, email, phoneLine, message))

	http.Redirect(w, r, "/contact?sent=1", http.StatusSeeOther)
}
