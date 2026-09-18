// Package models defines the domain types persisted by the portfolio site.
package models

import (
	"fmt"
	"strconv"
	"strings"
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
	ID          int64
	Category    string
	Level       string
	Achievement string
	Items       []string
	SortOrder   int
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
	ID                     int64
	UserID                 int64
	Name                   string
	Email                  string
	Message                string
	SlotStart              time.Time
	SlotEnd                time.Time
	Status                 string
	AmountCents            int
	Currency               string
	RazorpayOrderID        string
	RazorpayPaymentID      string
	PreviousSlotStart      *time.Time
	RescheduledAt          *time.Time
	RescheduleAcknowledged bool
	PromoCodeID            *int64
	PromoCode              string
	DiscountCents          int
	CreatedAt              time.Time
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
	Slug          string
	Title         string
	Summary       string
	SourceURL     string
	SourceName    string
	Tags          []string
	ThumbsUpCount int
	PublishedDate *time.Time
	IsPublished   bool
	SortOrder     int
	CreatedAt     time.Time
}

// TagsDisplay renders an article's tags as a comma-separated string, for
// the admin edit form's text input.
func (a Article) TagsDisplay() string {
	return strings.Join(a.Tags, ", ")
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
	UserID            int64
	Email             string
	AmountCents       int
	Currency          string
	Status            string
	RazorpayOrderID   string
	RazorpayPaymentID string
	AccessToken       string
	PromoCodeID       *int64
	PromoCode         string
	DiscountCents     int
	CreatedAt         time.Time
	PaidAt            *time.Time
}

const (
	PromoProductCourse      = "course"
	PromoProductConsultancy = "consultancy"
)

// PromoCode is a discount code an admin creates in the admin console and a
// visitor redeems at checkout. It applies to exactly one product line —
// courses (optionally scoped to specific courses, or all of them) or the
// single consultancy offering — never both.
type PromoCode struct {
	ID                  int64
	Code                string
	Description         string
	Product             string // PromoProductCourse | PromoProductConsultancy
	DiscountPercent     int
	AppliesToAllCourses bool
	CourseIDs           []int64
	MaxRedemptions      int // 0 means unlimited
	RedemptionCount     int
	ExpiresAt           time.Time
	IsActive            bool
	CreatedAt           time.Time
}

// HasCourse reports whether a course-scoped promo code applies to the
// given course. Irrelevant when AppliesToAllCourses is true.
func (p PromoCode) HasCourse(id int64) bool {
	for _, cid := range p.CourseIDs {
		if cid == id {
			return true
		}
	}
	return false
}

func (p PromoCode) IsExpired() bool { return time.Now().After(p.ExpiresAt) }

// IsExhausted reports whether the code has hit its total redemption cap;
// a cap of 0 means unlimited.
func (p PromoCode) IsExhausted() bool {
	return p.MaxRedemptions > 0 && p.RedemptionCount >= p.MaxRedemptions
}

// Usable reports whether the code can still be redeemed right now.
func (p PromoCode) Usable() bool {
	return p.IsActive && !p.IsExpired() && !p.IsExhausted()
}

// RedemptionsDisplay renders e.g. "3 / 50" or "3 / unlimited" for the
// admin promo list.
func (p PromoCode) RedemptionsDisplay() string {
	if p.MaxRedemptions == 0 {
		return strconv.Itoa(p.RedemptionCount) + " / unlimited"
	}
	return strconv.Itoa(p.RedemptionCount) + " / " + strconv.Itoa(p.MaxRedemptions)
}

// QuizPassPercent is the minimum score required to pass a course's final
// quiz and unlock a completion certificate.
const QuizPassPercent = 70

// QuizQuestion is one multiple-choice question in a course's final quiz.
type QuizQuestion struct {
	ID            int64
	CourseID      int64
	Question      string
	OptionA       string
	OptionB       string
	OptionC       string
	OptionD       string
	CorrectOption string // "A" | "B" | "C" | "D"
	SortOrder     int
}

// Option returns the text of the option labeled by the given letter
// ("A"-"D"), for rendering the quiz-taking form generically.
func (q QuizQuestion) Option(letter string) string {
	switch letter {
	case "A":
		return q.OptionA
	case "B":
		return q.OptionB
	case "C":
		return q.OptionC
	case "D":
		return q.OptionD
	}
	return ""
}

// QuizAttempt records one completed run through a course's final quiz —
// by a logged-in visitor (UserID set) or an admin previewing their own
// course (IsAdmin true, no UserID needed). CertificateCode is only set
// once Passed is true, and is what the certificate download is looked up
// by.
type QuizAttempt struct {
	ID              int64
	CourseID        int64
	UserID          *int64
	IsAdmin         bool
	Name            string
	Email           string
	TotalQuestions  int
	CorrectAnswers  int
	ScorePercent    int
	Passed          bool
	CertificateCode string
	CreatedAt       time.Time
}
