// Package models defines the domain types persisted by the portfolio site.
package models

import (
	"fmt"
	"sort"
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

// DefaultResumeKey is the role_resumes row for the main download. It is
// not a job, so the About page does not list it in the role select.
const DefaultResumeKey = "default"

// RoleResume is one downloadable PDF. RoleKey DefaultResumeKey is the
// resume served for every download that is not a stored role-specific
// file. Any other key is one role, shown in the select only when that
// file exists. The bytes live in the private resumes bucket.
type RoleResume struct {
	RoleKey    string
	Label      string
	ObjectPath string
	SortOrder  int
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

// Certification is one credential on the About page. Both dates are
// pointers because both are genuinely optional: a certification with no
// issue date on record should render without one rather than with a
// zero-value date that looks real, and a nil ExpiresOn means "does not
// expire", which is a different fact from "expires, date unknown".
type Certification struct {
	ID            int64
	Name          string
	Issuer        string
	IssuedOn      *time.Time
	ExpiresOn     *time.Time
	CredentialURL string
	IsPublished   bool
	SortOrder     int
	CreatedAt     time.Time
}

// IsExpired reports whether a certification's expiry has passed. A
// certification with no expiry never counts as expired.
func (c Certification) IsExpired() bool {
	return c.ExpiresOn != nil && c.ExpiresOn.Before(time.Now())
}

// Publication is one talk, podcast, guest article, or paper on the About
// page. Kind is free text ("Conference talk", "Podcast", …) shown as a
// label beside the title — see migration 046 for why it isn't an enum.
type Publication struct {
	ID          int64
	Title       string
	Kind        string
	Venue       string
	URL         string
	PublishedOn *time.Time
	Summary     string
	IsPublished bool
	SortOrder   int
	CreatedAt   time.Time
}

type Testimonial struct {
	ID          int64
	UserID      int64 // 0 if admin-authored rather than submitted by a visitor
	AuthorName  string
	AuthorTitle string
	Body        string
	IsPublished bool
	SortOrder   int
	CreatedAt   time.Time
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
	ContentObject    string // private Storage path, courses/{slug}.html
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// IsFree reports whether the course has no listed price.
func (c Course) IsFree() bool { return !c.IsPaid && c.PriceCents == 0 }

// The three rungs of the suggested learning path on /courses. Every course
// is priced identically ("contribute as you like"), so price carries no
// signal about relative depth — the level does, and grouping by it is what
// turns a flat list of eight titles into a route through them.
const (
	CourseTrackStart = "start" // opens at beginner level: a genuine entry point
	CourseTrackBuild = "build" // assumes some backend experience already
	CourseTrackDeep  = "deep"  // advanced throughout
)

// Track buckets a course onto one rung of that path, derived from its own
// Level string rather than stored in a second column — so a course added
// or re-levelled in the admin console lands on the right rung with nothing
// else to keep in sync. A course with no level set is treated as the
// middle rung, which is the safe default: it's neither promised as a
// beginner entry point nor hidden behind an "advanced" label.
func (c Course) Track() string {
	switch {
	case strings.HasPrefix(c.Level, "Beginner"):
		return CourseTrackStart
	case strings.HasPrefix(c.Level, "Advanced"):
		return CourseTrackDeep
	default:
		return CourseTrackBuild
	}
}

// PriceDisplay renders the price as a human string. Every paid course is
// "contribute as you like" by default (no hard price) — see
// RegionalPriceDisplay for the visitor-aware version (India gets its own
// INR minimum, when one is set).
func (c Course) PriceDisplay() string {
	if c.IsPaid {
		return contributeDisplay(c.MinPriceCents, c.Currency)
	}
	if c.IsFree() {
		return "Free"
	}
	return FormatMoney(c.PriceCents, c.Currency)
}

// contributeDisplay renders the "contribute as you like" pricing copy used
// everywhere a paid course's price would otherwise show: no minimum set
// (the default) reads as fully free with an optional contribution, and a
// minimum reads the same way with the floor called out after an asterisk.
func contributeDisplay(minCents int, currency string) string {
	if minCents <= 0 {
		return "Contribute as you like to support"
	}
	return "Contribute as you like to support *min " + FormatMoney(minCents, currency)
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

// RegionalPriceDisplay renders the "contribute as you like" pricing copy
// (with a "*min $X" suffix only when a minimum is actually set) for the
// given visitor's country — the region-aware counterpart to PriceDisplay,
// used wherever a course price is shown to an actual visitor.
func (c Course) RegionalPriceDisplay(country string) string {
	if !c.IsPaid {
		return c.PriceDisplay()
	}
	currency, cents := c.RegionalPrice(country)
	return contributeDisplay(cents, currency)
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

// FormatMoneyTotals joins per-currency totals the way the admin ledger
// does ("$12 + ₹500"). Empty or all-zero maps return "".
func FormatMoneyTotals(totals map[string]int) string {
	if len(totals) == 0 {
		return ""
	}
	currencies := make([]string, 0, len(totals))
	for currency, cents := range totals {
		if cents > 0 {
			currencies = append(currencies, currency)
		}
	}
	sort.Strings(currencies)
	if len(currencies) == 0 {
		return ""
	}
	parts := make([]string, 0, len(currencies))
	for _, currency := range currencies {
		parts = append(parts, FormatMoney(totals[currency], currency))
	}
	return strings.Join(parts, " + ")
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
	Phone                  string
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

// ContactMessage is a submission from the /contact page's form — a
// simple lead capture (name, email, phone, message), no payment or
// scheduling involved. Read-only from the visitor's side once sent;
// IsRead lets the admin track which ones they've already seen.
type ContactMessage struct {
	ID        int64
	Name      string
	Email     string
	Phone     string
	Message   string
	IsRead    bool
	CreatedAt time.Time
}

// Consultancy is three tiers, none of them self-checkout: a fixed-price
// (reference only, see below) Architecture Review and a monthly retainer
// are both scoped on a call via /contact, and hourly booking is a free
// scheduled call via /booking with no price at all — no payment gateway
// is wired up anywhere in consultancy right now (see
// docs/ARCHITECTURE.md §6). AuditPriceCentsUSD/INR is kept as a
// reference figure shown on the page ("Discuss an Architecture Review —
// $3,000 · scoped on a call"), not something charged against; there is
// no equivalent constant for the hourly session since it has no price
// to display anymore.
const (
	ConsultancyDurationMinutes = 60

	// AuditPriceCentsUSD/INR is a reference figure only — shown on the
	// consultancy page, never charged. The Architecture Review is scoped
	// and invoiced outside this site.
	AuditPriceCentsUSD = 300000   // $3,000
	AuditPriceCentsINR = 15000000 // ₹1,50,000

	// RetainerPriceRangeUSD/INR are display-only ranges shown on the
	// consultancy page — a retainer is scoped on a call, never
	// self-checkout, so there's no fixed cents constant to charge against.
	RetainerPriceRangeUSD = "$4,000–$8,000/month"
	RetainerPriceRangeINR = "₹2,00,000–₹4,00,000/month"
)

// AuditRegionalPrice mirrors Course.RegionalPrice for the reference-only
// Architecture Review figure.
func AuditRegionalPrice(country string) (currency string, cents int) {
	if country == "IN" {
		return "INR", AuditPriceCentsINR
	}
	return "USD", AuditPriceCentsUSD
}

func AuditRegionalPriceDisplay(country string) string {
	currency, cents := AuditRegionalPrice(country)
	return FormatMoney(cents, currency)
}

// RetainerRangeDisplay renders the display-only monthly retainer range for
// the visitor's region.
func RetainerRangeDisplay(country string) string {
	if country == "IN" {
		return RetainerPriceRangeINR
	}
	return RetainerPriceRangeUSD
}

// AuditRequest is a paid, fixed-scope Architecture Review / technical due
// diligence engagement — a single fixed-price checkout (unlike a course's
// pay-what-you-want model), so it's modeled much closer to Booking than to
// Purchase: one row per inquiry, moving from pending_payment to paid.
type AuditRequest struct {
	ID                int64
	Name              string
	Email             string
	Company           string
	Brief             string
	AmountCents       int
	Currency          string
	Status            string
	RazorpayOrderID   string
	RazorpayPaymentID string
	CreatedAt         time.Time
	PaidAt            *time.Time
}

const (
	AuditStatusPendingPayment = "pending_payment"
	AuditStatusPaid           = "paid"
)

type User struct {
	ID           int64
	Email        string
	PasswordHash string
	Name         string
	IsGuest      bool
	CreatedAt    time.Time
}

// Article is one of Vishnu's own blog posts, written and hosted here.
// Body is the post itself — plain text or HTML, see articleBody in
// core/view/view.go. The schema used to carry source_url/source_name for a
// second kind of article that was only a link to something published
// elsewhere; that feature is gone (migration 040) and an article is
// original content with a body, full stop.
type Article struct {
	ID            int64
	Slug          string
	Title         string
	Summary       string
	Body          string
	ContentObject string // private Storage path, articles/{slug}.html; empty means Body is the post
	CoverImageURL string
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

// ArticleTag is one topic on the blog plus how many published articles
// carry it. The count is what turns a flat list of tags into a visible
// topic structure — a reader can see which subjects the archive actually
// goes deep on rather than guessing from a dropdown.
type ArticleTag struct {
	Name  string
	Count int
}

// ArticleVote is one thumbs-up on an article, for the admin "who voted"
// view. UserName/UserEmail are empty when the voter wasn't logged in —
// voting never requires an account, so an anonymous vote is expected,
// not an error.
type ArticleVote struct {
	ID        int64
	UserName  string
	UserEmail string
	// IsGuest is true when UserEmail is set but the account was created
	// from the anonymous-vote form's optional email field, not an actual
	// sign-in — distinct from a real logged-in visitor's vote.
	IsGuest   bool
	CreatedAt time.Time
}

// IsAnonymous reports whether this vote came with no name or email at
// all — the visitor left both fields on the vote form blank.
func (v ArticleVote) IsAnonymous() bool { return v.UserEmail == "" && v.UserName == "" }

const (
	ContributionStatusCreated = "created"
	ContributionStatusPaid    = "paid"
	ContributionStatusFailed  = "failed"
	// ArticleContributeMinCents is the smallest charge Razorpay will take
	// (one whole currency unit). The box is optional — a visitor who does
	// not want to pay just leaves it.
	ArticleContributeMinCents = 100
)

// ArticleContribution is one optional payment on an article. The same
// visitor may contribute more than once; each row is its own payment
// with its own paid_at.
type ArticleContribution struct {
	ID                int64
	ArticleID         int64
	UserID            int64
	Email             string
	AmountCents       int
	Currency          string
	Status            string
	RazorpayOrderID   string
	RazorpayPaymentID string
	CreatedAt         time.Time
	PaidAt            *time.Time
}

// ContributionCurrency is the checkout currency for optional article
// support: INR for India, otherwise the site's configured fallback
// (CHECKOUT_CURRENCY, usually USD).
func ContributionCurrency(country, fallback string) string {
	if country == "IN" {
		return "INR"
	}
	if fallback == "" {
		return "USD"
	}
	return fallback
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

// AdminMessage is a note the admin sent a specific visitor account,
// surfaced on their /account page and in the header's notification menu
// until read.
type AdminMessage struct {
	ID        int64
	UserID    int64
	Body      string
	IsRead    bool
	CreatedAt time.Time
}
