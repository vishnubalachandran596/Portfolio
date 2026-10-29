package handlers

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/scheduler"
	"github.com/vishnubalachandran596/portfolio/core/view"
)

// Render smoke test: a mid-render template error (for example a data field
// that shadows BaseData.Active, which broke the manage page once) is invisible
// to a bare parse, so execute the public booking pages with realistic data.
func TestPublicBookingTemplatesRender(t *testing.T) {
	r, err := view.New()
	if err != nil {
		t.Fatal(err)
	}
	base := BaseData{SiteName: "Vishnu Balachandran", Active: "consultancy",
		Nav: NavLinks{Home: "/", Courses: "/courses", Software: "/software", Consultancy: "/consultancy"}}
	loc := testIST()
	start := futureAt(loc, 2, 11)
	b := models.Booking{ID: 1, Name: "Ada", Code: "BK-ABC234", Status: models.BookingStatusConfirmed, SlotStart: start, SlotEnd: start.Add(time.Hour)}
	days := scheduler.ComputeDays(allWeekRules("10:00", "20:00"), map[time.Time]bool{}, loc, 3)
	manage := BookingManageData{BaseData: base, Token: "abc", Booking: b, When: "Wed", Duration: 60, CanChange: true, Notice: "ok", Days: days}

	cases := []struct {
		page string
		data any
	}{
		{"booking", BookingData{BaseData: base, Days: days, LoggedIn: true, UserEmail: "a@b.test", UserName: "Ada"}},
		{"booking", BookingData{BaseData: base, Days: days}}, // logged out: login prompt, no picker
		{"booking", BookingData{BaseData: base}},
		{"contact", ContactData{BaseData: base}},
		{"contact", ContactData{BaseData: base, LoggedIn: true, Name: "Ada", Email: "a@b.test"}},
		{"booking_manage", manage},
		{"booking_manage_reschedule", manage},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		r.Render(rec, 200, c.page, c.data)
		body := rec.Body.String()
		if strings.Contains(body, "<!-- render error:") || !strings.Contains(body, "</html>") {
			t.Fatalf("%s: %s", c.page, body[max(0, len(body)-300):])
		}
	}
}

// A logged-out visitor sees the log-in prompt and no slot picker or form.
func TestBookingPageLoggedOutHasNoForm(t *testing.T) {
	r, err := view.New()
	if err != nil {
		t.Fatal(err)
	}
	base := BaseData{SiteName: "Vishnu Balachandran", Active: "consultancy"}
	days := scheduler.ComputeDays(allWeekRules("10:00", "20:00"), map[time.Time]bool{}, testIST(), 3)
	rec := httptest.NewRecorder()
	r.Render(rec, 200, "booking", BookingData{BaseData: base, Days: days})
	body := rec.Body.String()
	if !strings.Contains(body, `id="booking-login"`) || strings.Contains(body, `id="booking-picker"`) || strings.Contains(body, "slot-btn") {
		t.Fatal("logged-out booking page must show the log-in prompt only")
	}
}

// The new account pages render with realistic data.
func TestAuthEmailTemplatesRender(t *testing.T) {
	r, err := view.New()
	if err != nil {
		t.Fatal(err)
	}
	base := BaseData{SiteName: "Vishnu Balachandran"}
	cases := []struct {
		page string
		data PasswordAuthData
	}{
		{"signup", PasswordAuthData{BaseData: base, Next: "/booking", GoogleEnabled: true}},
		{"forgot", PasswordAuthData{BaseData: base, Email: "a@b.test", Error: "x"}},
		{"reset_password", PasswordAuthData{BaseData: base, Email: "a@b.test", Token: "tok"}},
		{"account_security", PasswordAuthData{BaseData: base, HasPassword: true}},
		{"auth_message", PasswordAuthData{BaseData: base, Heading: "H", Message: "M", Links: []AuthLink{{Label: "Go", Href: "/x", Primary: true}, {Label: "Other", Href: "/y"}}}},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		r.Render(rec, 200, c.page, c.data)
		body := rec.Body.String()
		if strings.Contains(body, "<!-- render error:") || !strings.Contains(body, "</html>") {
			t.Fatalf("%s: %s", c.page, body[max(0, len(body)-300):])
		}
		if strings.Contains(strings.ToLower(body), "security question") {
			t.Fatalf("%s still mentions the security question", c.page)
		}
	}
}
