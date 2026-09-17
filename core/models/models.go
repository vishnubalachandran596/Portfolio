// Package models defines the domain types persisted by the portfolio site.
package models

import (
	"fmt"
	"strconv"
	"time"
)

type Admin struct {
	ID           int64
	Email        string
	PasswordHash string
	Name         string
	CreatedAt    time.Time
}

type Profile struct {
	Name            string
	Title           string
	Tagline         string
	HeroSubtitle    string
	Summary         string
	Location        string
	Email           string
	LinkedInURL     string
	GitHubURL       string
	ResumeURL       string
	PhotoURL        string
	YearsExperience int
	UpdatedAt       time.Time
}

type SkillGroup struct {
	ID        int64
	Category  string
	Level     string
	Items     []string
	SortOrder int
}

type Experience struct {
	ID        int64
	Company   string
	Role      string
	Location  string
	StartDate string
	EndDate   string
	Bullets   []string
	SortOrder int
}

type Education struct {
	ID          int64
	Institution string
	Degree      string
	Field       string
	StartYear   int
	EndYear     int
	Details     string
	SortOrder   int
}

type HomeFeature struct {
	ID          int64
	Title       string
	Description string
	SortOrder   int
}

type Course struct {
	ID               int64
	Slug             string
	Title            string
	Summary          string
	Description      string
	Level            string
	Duration         string
	PriceCents       int
	Currency         string
	ImageURL         string
	IsPublished      bool
	SortOrder        int
	IsPaid           bool
	MinPriceCents    int
	MinPriceCentsINR int
	ContentKey       string
	ContentHTML      string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// IsFree reports whether the course has no listed price.
func (c Course) IsFree() bool { return !c.IsPaid && c.PriceCents == 0 }

// PriceDisplay renders the price as a human string, e.g. "$49" or "Free".
// For a paid course this shows the international minimum — see
// RegionalPrice for the visitor-aware version (India gets an INR floor).
func (c Course) PriceDisplay() string {
	if c.IsPaid {
		return "From " + FormatMoney(c.MinPriceCents, c.Currency)
	}
	if c.IsFree() {
		return "Free"
	}
	return FormatMoney(c.PriceCents, c.Currency)
}

// MinPriceDisplay renders the minimum contribution for a paid course.
func (c Course) MinPriceDisplay() string { return FormatMoney(c.MinPriceCents, c.Currency) }

// MinPriceWhole is the minimum contribution in whole currency units
// (e.g. 5 for $5), for use as a numeric HTML input attribute.
func (c Course) MinPriceWhole() int { return c.MinPriceCents / 100 }

// RegionalPrice returns the currency and minimum contribution (in the
// smallest currency unit) that applies for a visitor from the given ISO
// 3166-1 alpha-2 country code: India gets its own INR minimum (when set),
// everyone else — including unknown/local visitors — gets the course's
// default international minimum.
func (c Course) RegionalPrice(country string) (currency string, minCents int) {
	if country == "IN" && c.MinPriceCentsINR > 0 {
		return "INR", c.MinPriceCentsINR
	}
	return c.Currency, c.MinPriceCents
}

// RegionalPriceDisplay renders "From ₹500" / "From $5" / "Free" for the
// given visitor's country — the region-aware counterpart to PriceDisplay,
// used wherever a course price is shown to an actual visitor.
func (c Course) RegionalPriceDisplay(country string) string {
	if !c.IsPaid {
		return c.PriceDisplay()
	}
	currency, cents := c.RegionalPrice(country)
	return "From " + FormatMoney(cents, currency)
}

func FormatMoney(cents int, currency string) string {
	symbol := "$"
	switch currency {
	case "EUR":
		symbol = "€"
	case "GBP":
		symbol = "£"
	case "INR":
		symbol = "₹"
	}
	whole := cents / 100
	remainder := cents % 100
	if remainder == 0 {
		return symbol + strconv.Itoa(whole)
	}
	return symbol + strconv.Itoa(whole) + fmt.Sprintf(".%02d", remainder)
}

const (
	BookingStatusPendingPayment = "pending_payment"
	BookingStatusConfirmed      = "confirmed"
	BookingStatusCancelled      = "cancelled"
)

type Booking struct {
	ID                int64
	UserID            int64
	Name              string
	Email             string
	Message           string
	SlotStart         time.Time
	SlotEnd           time.Time
	Status            string
	AmountCents       int
	Currency          string
	RazorpayOrderID   string
	RazorpayPaymentID string
	CreatedAt         time.Time
}

// Consultancy is a single paid 1:1 offering (not stored per-row like a
// course — one product, priced by region the same way a paid course is).
const (
	ConsultancyDurationMinutes = 60
	ConsultancyPriceCentsUSD   = 4000   // $40/hour
	ConsultancyPriceCentsINR   = 300000 // ₹3,000/hour
)

// ConsultancyRegionalPrice mirrors Course.RegionalPrice: India gets its
// own INR rate, everyone else the USD rate.
func ConsultancyRegionalPrice(country string) (currency string, cents int) {
	if country == "IN" {
		return "INR", ConsultancyPriceCentsINR
	}
	return "USD", ConsultancyPriceCentsUSD
}

// ConsultancyRegionalPriceDisplay renders "₹3,000/hour" or "$40/hour" for
// the given visitor's country.
func ConsultancyRegionalPriceDisplay(country string) string {
	currency, cents := ConsultancyRegionalPrice(country)
	return FormatMoney(cents, currency) + "/hour"
}

type User struct {
	ID           int64
	Email        string
	PasswordHash string
	Name         string
	CreatedAt    time.Time
}

// Article is a curated link to an external piece with a short in-house
// summary — added by the admin pasting a URL and writing the summary,
// not scraped/re-hosted content.
type Article struct {
	ID            int64
	Title         string
	Summary       string
	SourceURL     string
	SourceName    string
	PublishedDate *time.Time
	IsPublished   bool
	SortOrder     int
	CreatedAt     time.Time
}

type AvailabilityRule struct {
	ID          int64
	Weekday     time.Weekday
	StartTime   string // "HH:MM"
	EndTime     string // "HH:MM"
	SlotMinutes int
	Active      bool
}

// TimeSlot is a bookable slot computed from availability rules minus
// existing bookings, shown to visitors on the booking page.
type TimeSlot struct {
	Start     time.Time
	End       time.Time
	Available bool
}

const (
	PurchaseStatusCreated = "created"
	PurchaseStatusPaid    = "paid"
	PurchaseStatusFailed  = "failed"
)

// Purchase tracks a Razorpay order for a paid course, from creation
// through payment verification. AccessToken is the opaque bearer
// credential (set as a cookie, or usable as a magic-link query param)
// that unlocks the course's gated content once Status is "paid".
type Purchase struct {
	ID                int64
	CourseID          int64
	Email             string
	AmountCents       int
	Currency          string
	Status            string
	RazorpayOrderID   string
	RazorpayPaymentID string
	AccessToken       string
	CreatedAt         time.Time
	PaidAt            *time.Time
}
