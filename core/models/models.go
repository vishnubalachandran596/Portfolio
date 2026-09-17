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
	ID            int64
	Slug          string
	Title         string
	Summary       string
	Description   string
	Level         string
	Duration      string
	PriceCents    int
	Currency      string
	ImageURL      string
	IsPublished   bool
	SortOrder     int
	IsPaid        bool
	MinPriceCents int
	ContentKey    string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// IsFree reports whether the course has no listed price.
func (c Course) IsFree() bool { return !c.IsPaid && c.PriceCents == 0 }

// PriceDisplay renders the price as a human string, e.g. "$49" or "Free".
func (c Course) PriceDisplay() string {
	if c.IsPaid {
		return "From " + formatMoney(c.MinPriceCents, c.Currency)
	}
	if c.IsFree() {
		return "Free"
	}
	return formatMoney(c.PriceCents, c.Currency)
}

// MinPriceDisplay renders the minimum contribution for a paid course.
func (c Course) MinPriceDisplay() string { return formatMoney(c.MinPriceCents, c.Currency) }

// MinPriceWhole is the minimum contribution in whole currency units
// (e.g. 5 for $5), for use as a numeric HTML input attribute.
func (c Course) MinPriceWhole() int { return c.MinPriceCents / 100 }

func formatMoney(cents int, currency string) string {
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
	BookingStatusPending   = "pending"
	BookingStatusConfirmed = "confirmed"
	BookingStatusCancelled = "cancelled"
)

type Booking struct {
	ID        int64
	Name      string
	Email     string
	Message   string
	SlotStart time.Time
	SlotEnd   time.Time
	Status    string
	CreatedAt time.Time
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
