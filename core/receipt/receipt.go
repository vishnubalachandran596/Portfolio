// Package receipt renders a plain payment receipt as a PDF, generated
// in-process with a pure-Go PDF library — the same approach as the course
// certificates and resume. It's a receipt of what was paid, not a formal
// tax invoice (no GST/VAT registration details), but it's a real,
// downloadable record a buyer can keep or forward for expense reporting.
package receipt

import (
	"bytes"
	"strconv"
	"time"

	"github.com/go-pdf/fpdf"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

type Data struct {
	ReceiptNo   string
	IssuedAt    time.Time
	PayerEmail  string
	Description string
	AmountCents int
	Currency    string
	// PromoCode and DiscountCents are empty/zero when no promo code was
	// applied. AmountCents above is always the amount actually paid
	// (already net of the discount) — these two only add a visible line
	// showing that a code was used and what it saved.
	PromoCode     string
	DiscountCents int
	PaymentID     string
	SiteName      string
	IssuerName    string
}

// Generate renders the receipt and returns the PDF bytes.
func Generate(d Data) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(20, 20, 20)
	pdf.SetAutoPageBreak(true, 20)
	pdf.AddPage()

	const contentW = 210.0 - 40.0

	pdf.SetFont("Helvetica", "B", 18)
	pdf.SetTextColor(30, 30, 30)
	pdf.CellFormat(contentW, 9, d.IssuerName, "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetTextColor(100, 100, 100)
	pdf.CellFormat(contentW, 6, d.SiteName, "", 1, "L", false, 0, "")
	pdf.Ln(6)

	pdf.SetFont("Helvetica", "B", 14)
	pdf.SetTextColor(30, 30, 30)
	pdf.CellFormat(contentW, 8, "Payment Receipt", "", 1, "L", false, 0, "")
	pdf.Ln(2)

	row := func(label, value string) {
		pdf.SetFont("Helvetica", "", 10)
		pdf.SetTextColor(100, 100, 100)
		pdf.CellFormat(45, 6.5, label, "", 0, "L", false, 0, "")
		pdf.SetFont("Helvetica", "B", 10)
		pdf.SetTextColor(30, 30, 30)
		pdf.CellFormat(contentW-45, 6.5, value, "", 1, "L", false, 0, "")
	}

	row("Receipt No.", d.ReceiptNo)
	row("Date", d.IssuedAt.Format("Jan 2, 2006"))
	row("Billed to", d.PayerEmail)
	if d.PaymentID != "" {
		row("Payment ID", d.PaymentID)
	}
	pdf.Ln(6)

	y := pdf.GetY()
	pdf.SetDrawColor(210, 210, 210)
	pdf.SetLineWidth(0.3)
	pdf.Line(20, y, 190, y)
	pdf.Ln(4)

	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetTextColor(100, 100, 100)
	pdf.CellFormat(contentW-35, 6.5, "DESCRIPTION", "", 0, "L", false, 0, "")
	pdf.CellFormat(35, 6.5, "AMOUNT", "", 1, "R", false, 0, "")
	pdf.Ln(1)

	pdf.SetFont("Helvetica", "", 10.5)
	pdf.SetTextColor(30, 30, 30)
	pdf.CellFormat(contentW-35, 6.5, d.Description, "", 0, "L", false, 0, "")
	pdf.CellFormat(35, 6.5, formatMoney(d.AmountCents, d.Currency), "", 1, "R", false, 0, "")

	if d.PromoCode != "" {
		pdf.Ln(6.5)
		pdf.SetFont("Helvetica", "", 9.5)
		pdf.SetTextColor(100, 100, 100)
		pdf.CellFormat(contentW-35, 6, "Promo code applied: "+d.PromoCode, "", 0, "L", false, 0, "")
		pdf.CellFormat(35, 6, "-"+formatMoney(d.DiscountCents, d.Currency), "", 1, "R", false, 0, "")
	}
	pdf.Ln(4)

	y = pdf.GetY()
	pdf.Line(20, y, 190, y)
	pdf.Ln(4)

	pdf.SetFont("Helvetica", "B", 11)
	pdf.SetTextColor(30, 30, 30)
	pdf.CellFormat(contentW-35, 7, "Total Paid", "", 0, "L", false, 0, "")
	pdf.CellFormat(35, 7, formatMoney(d.AmountCents, d.Currency), "", 1, "R", false, 0, "")
	pdf.Ln(10)

	pdf.SetFont("Helvetica", "I", 8.5)
	pdf.SetTextColor(140, 140, 140)
	pdf.MultiCell(contentW, 4.5, "This is a receipt of payment received, not a formal tax invoice. Payment processed and verified via Razorpay. A paid course purchase includes a free 1-hour consultation call -- send this receipt via LinkedIn or email to redeem it.", "", "L", false)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func formatMoney(cents int, currency string) string {
	return models.FormatMoney(cents, currency)
}

func receiptNo(prefix string, id int64) string {
	return prefix + "-" + strconv.FormatInt(id, 10)
}

// ReceiptNoForBooking and ReceiptNoForPurchase give each receipt type its
// own numbering namespace so a booking and a course purchase never collide
// on the same receipt number even if their row IDs happen to match.
func ReceiptNoForBooking(id int64) string  { return receiptNo("CNS", id) }
func ReceiptNoForPurchase(id int64) string { return receiptNo("CRS", id) }
func ReceiptNoForArticleContribution(id int64) string {
	return receiptNo("ART", id)
}
