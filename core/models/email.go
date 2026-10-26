package models

import "time"

// Transactional email kinds. Newsletter campaign blasts are not a kind:
// only the signup confirmation is sent.
const (
	EmailPurchaseCourse     = "purchase_course"
	EmailPurchaseSoftware   = "purchase_software"
	EmailPurchaseArticle    = "purchase_article"
	EmailContribution       = "contribution"
	EmailBookingNew         = "booking_new"
	EmailBookingRescheduled = "booking_rescheduled"
	EmailBookingCancelled   = "booking_cancelled"
	EmailConnectRequest     = "connect_request"
	EmailNewsletterConfirm  = "newsletter_confirm"
	EmailSendTest           = "send_test"
)

const (
	EmailStatusPending  = "pending"
	EmailStatusSent     = "sent"
	EmailStatusFailed   = "failed"
	EmailStatusDisabled = "disabled"
)

// EmailKinds is every customer-facing template, in admin-list order.
// Send-test is an outbox row, not a template.
var EmailKinds = []string{
	EmailPurchaseCourse,
	EmailPurchaseSoftware,
	EmailPurchaseArticle,
	EmailContribution,
	EmailBookingNew,
	EmailBookingRescheduled,
	EmailBookingCancelled,
	EmailConnectRequest,
	EmailNewsletterConfirm,
}

// EmailKindLabel is the admin console name for a kind.
func EmailKindLabel(kind string) string {
	switch kind {
	case EmailPurchaseCourse:
		return "Course purchase"
	case EmailPurchaseSoftware:
		return "Software purchase"
	case EmailPurchaseArticle:
		return "Article contribution"
	case EmailContribution:
		return "Course contribution"
	case EmailBookingNew:
		return "Booking confirmed"
	case EmailBookingRescheduled:
		return "Booking rescheduled"
	case EmailBookingCancelled:
		return "Booking cancelled"
	case EmailConnectRequest:
		return "Consultancy connect"
	case EmailNewsletterConfirm:
		return "Newsletter confirmation"
	default:
		return kind
	}
}

// EmailUsesResend reports whether Resend sends this kind. Course, software,
// article, and contribution mail use Resend, including a zero amount.
// Bookings, consultancy connect requests, and the newsletter confirmation
// use Brevo.
func EmailUsesResend(kind string) bool {
	switch kind {
	case EmailPurchaseCourse, EmailPurchaseSoftware, EmailPurchaseArticle, EmailContribution:
		return true
	default:
		return false
	}
}

// SubjectFor is the built-in subject. A database template may replace it.
func SubjectFor(kind string) string {
	switch kind {
	case EmailPurchaseCourse:
		return "Your course purchase"
	case EmailPurchaseSoftware:
		return "Your software purchase"
	case EmailPurchaseArticle:
		return "Thank you for supporting this article"
	case EmailContribution:
		return "Thank you for your contribution"
	case EmailBookingNew:
		return "Your booking is confirmed"
	case EmailBookingRescheduled:
		return "Your booking was rescheduled"
	case EmailBookingCancelled:
		return "Your booking was cancelled"
	case EmailConnectRequest:
		return "We received your message"
	case EmailNewsletterConfirm:
		return "You are subscribed"
	default:
		return "Message from Vishnu Balachandran"
	}
}

// EmailFields is the placeholder whitelist for a kind. Templates may use
// only these names.
func EmailFields(kind string) []string {
	support := []string{"SiteName", "SupportReplyTo"}
	switch kind {
	case EmailPurchaseCourse, EmailPurchaseSoftware, EmailPurchaseArticle, EmailContribution:
		return append([]string{"BuyerName", "ItemTitle", "Amount", "ReceiptNo", "ReceiptURL", "AccessURL", "ClaimCode", "Instructions"}, support...)
	case EmailBookingNew, EmailBookingRescheduled, EmailBookingCancelled:
		return append([]string{"BuyerName", "ItemTitle", "BookingStart", "BookingCode", "Duration", "ManageURL"}, support...)
	case EmailConnectRequest:
		return append([]string{"BuyerName", "ManageURL"}, support...)
	case EmailNewsletterConfirm:
		return append([]string{"BuyerName", "UnsubscribeURL"}, support...)
	default:
		return nil
	}
}

// EmailData is the rendered template's data. Amount is already passed
// through FormatMoney. Empty strings are fine; templates hide them.
type EmailData struct {
	BuyerName      string
	ItemTitle      string
	Amount         string
	ReceiptNo      string
	ReceiptURL     string
	AccessURL      string
	ClaimCode      string
	Instructions   string
	BookingStart   string
	BookingCode    string
	Duration       string
	ManageURL      string
	UnsubscribeURL string
	SiteName       string
	SupportReplyTo string
}

// EmailOutbox is delivery metadata. The rendered body is never stored.
type EmailOutbox struct {
	ID         int64
	Kind       string
	EntityID   string
	ToEmail    string
	Provider   string
	Status     string
	Attempts   int
	LastError  string
	ProviderID string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	SentAt     *time.Time
}

// EmailTemplate is an admin override for one kind. No row means the
// built-in template is used.
type EmailTemplate struct {
	Kind      string
	Subject   string
	HTML      string
	TextBody  string
	UpdatedAt time.Time
	UpdatedBy string
}

// EmailTemplateVersion is one prior save, kept for restore.
type EmailTemplateVersion struct {
	ID       int64
	Kind     string
	Subject  string
	HTML     string
	TextBody string
	SavedAt  time.Time
	SavedBy  string
}
