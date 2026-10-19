// Package certificate renders a course certificate as a landscape A4 PDF,
// generated in-process with a pure-Go PDF library (no headless browser or
// external service needed, so it works fine inside a Vercel Go serverless
// function). It is designed to print well: white page, dark text, thin
// rules, no filled areas. All text is plain ASCII (see ascii.go).
package certificate

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
)

// Data is what a certificate states. Every text field is passed through
// the ASCII rule by Prepare before it is drawn.
type Data struct {
	RecipientName string
	CourseTitle   string
	IssuedAt      time.Time
	Code          string // stored form, 16 hex characters
	SiteName      string
	IssuerName    string
	ScorePercent  int
	PassPercent   int
	// VerifyURL is the public verification page for Code, printed on the
	// certificate so anyone can confirm it.
	VerifyURL string
}

// compress is switched off by tests so the page text can be inspected.
var compress = true

// Prepare returns d with every text field reduced to plain ASCII and
// defaults filled in, so the drawing code never sees a character a core
// PDF font cannot print.
func Prepare(d Data) Data {
	d.RecipientName = RecipientName(d.RecipientName, "")
	d.CourseTitle = SanitizeText(d.CourseTitle)
	if d.CourseTitle == "" {
		d.CourseTitle = "the course"
	}
	d.SiteName = SanitizeText(d.SiteName)
	d.IssuerName = SanitizeText(d.IssuerName)
	if d.IssuerName == "" {
		d.IssuerName = d.SiteName
	}
	d.VerifyURL = SanitizeText(d.VerifyURL)
	if n := NormalizeCode(d.Code); n != "" {
		d.Code = n
	}
	if d.PassPercent <= 0 {
		d.PassPercent = 70
	}
	return d
}

// DateText formats the issue date as plain ASCII, e.g. "05 March 2026".
func DateText(t time.Time) string { return t.Format("02 January 2006") }

var (
	navy = [3]int{27, 38, 59}
	gold = [3]int{150, 105, 20}
	grey = [3]int{90, 98, 110}
)

// Generate renders the certificate and returns the PDF bytes.
func Generate(d Data) ([]byte, error) {
	d = Prepare(d)

	pdf := fpdf.New("L", "mm", "A4", "")
	pdf.SetMargins(0, 0, 0)
	pdf.SetAutoPageBreak(false, 0)
	pdf.SetCompression(compress)
	pdf.SetTitle("Certificate - "+d.CourseTitle, false)
	pdf.SetAuthor(d.IssuerName, false)
	pdf.SetSubject("Certificate "+FormatCode(d.Code), false)
	pdf.SetCreator(d.SiteName, false)
	pdf.AddPage()

	const w, h = 297.0, 210.0

	setDraw := func(c [3]int) { pdf.SetDrawColor(c[0], c[1], c[2]) }
	setText := func(c [3]int) { pdf.SetTextColor(c[0], c[1], c[2]) }
	center := func(y, lineH float64, txt, font, style string, size float64, c [3]int) {
		pdf.SetFont(font, style, size)
		setText(c)
		pdf.SetXY(0, y)
		pdf.CellFormat(w, lineH, txt, "", 0, "C", false, 0, "")
	}

	// Frame: a firm outer rule, a fine inner rule, and gold corner brackets.
	setDraw(navy)
	pdf.SetLineWidth(1.0)
	pdf.Rect(8, 8, w-16, h-16, "D")
	setDraw(gold)
	pdf.SetLineWidth(0.3)
	pdf.Rect(11, 11, w-22, h-22, "D")
	pdf.SetLineWidth(0.9)
	const arm = 14.0
	for _, c := range [][4]float64{{15, 15, 1, 1}, {w - 15, 15, -1, 1}, {15, h - 15, 1, -1}, {w - 15, h - 15, -1, -1}} {
		pdf.Line(c[0], c[1], c[0]+arm*c[2], c[1])
		pdf.Line(c[0], c[1], c[0], c[1]+arm*c[3])
	}

	// Heading.
	center(24, 6, strings.ToUpper(d.IssuerName), "Helvetica", "B", 11, gold)
	center(33, 16, "CERTIFICATE", "Times", "B", 42, navy)
	center(50, 7, "OF ACHIEVEMENT", "Helvetica", "", 13, grey)

	// Divider with a small diamond.
	setDraw(gold)
	pdf.SetLineWidth(0.4)
	pdf.Line(88, 63, w/2-4, 63)
	pdf.Line(w/2+4, 63, w-88, 63)
	pdf.SetFillColor(gold[0], gold[1], gold[2])
	pdf.Polygon([]fpdf.PointType{{X: w / 2, Y: 60.5}, {X: w/2 + 2.5, Y: 63}, {X: w / 2, Y: 65.5}, {X: w/2 - 2.5, Y: 63}}, "F")

	// Body.
	center(71, 7, "This is to certify that", "Times", "I", 15, grey)

	nameSize := 34.0
	pdf.SetFont("Times", "BI", nameSize)
	for pdf.GetStringWidth(d.RecipientName) > w-70 && nameSize > 18 {
		nameSize -= 2
		pdf.SetFont("Times", "BI", nameSize)
	}
	center(81, 15, d.RecipientName, "Times", "BI", nameSize, navy)
	setDraw(gold)
	pdf.SetLineWidth(0.3)
	pdf.Line(60, 99, w-60, 99)

	center(104, 7, "has cleared the final quiz and is certified in", "Times", "I", 15, grey)

	// Course title, wrapped if long.
	titleSize := 21.0
	pdf.SetFont("Helvetica", "B", titleSize)
	lines := pdf.SplitText(d.CourseTitle, w-80)
	for len(lines) > 2 && titleSize > 13 {
		titleSize -= 1.5
		pdf.SetFont("Helvetica", "B", titleSize)
		lines = pdf.SplitText(d.CourseTitle, w-80)
	}
	y := 114.0
	if len(lines) > 1 {
		y = 111
	}
	for _, ln := range lines {
		center(y, 10, strings.TrimSpace(ln), "Helvetica", "B", titleSize, navy)
		y += 10
	}
	center(y+2, 6, fmt.Sprintf("Final quiz score %d percent (pass mark %d percent)", d.ScorePercent, d.PassPercent), "Helvetica", "", 10.5, grey)

	// Footer: date (left), seal (centre), signature (right).
	const fy = 165.0
	setDraw(navy)
	pdf.SetLineWidth(0.3)
	pdf.Line(30, fy, 95, fy)
	pdf.Line(w-95, fy, w-30, fy)
	pdf.SetFont("Helvetica", "B", 12)
	setText(navy)
	pdf.SetXY(30, fy-8)
	pdf.CellFormat(65, 6, DateText(d.IssuedAt), "", 0, "C", false, 0, "")
	pdf.SetXY(w-95, fy-8)
	pdf.CellFormat(65, 6, d.IssuerName, "", 0, "C", false, 0, "")
	pdf.SetFont("Helvetica", "", 9.5)
	setText(grey)
	pdf.SetXY(30, fy+1.5)
	pdf.CellFormat(65, 5, "Date of issue", "", 0, "C", false, 0, "")
	pdf.SetXY(w-95, fy+1.5)
	pdf.CellFormat(65, 5, "Instructor", "", 0, "C", false, 0, "")

	// Seal: concentric rings with CERTIFIED across the middle.
	setDraw(gold)
	pdf.SetLineWidth(0.9)
	pdf.Circle(w/2, fy-6, 15, "D")
	pdf.SetLineWidth(0.3)
	pdf.Circle(w/2, fy-6, 12.5, "D")
	center(fy-9.5, 4, "CERTIFIED", "Helvetica", "B", 8, gold)
	center(fy-4.5, 4, fmt.Sprintf("%d", d.IssuedAt.Year()), "Helvetica", "", 8, gold)

	// Code and verification line.
	center(fy+18, 5, "Certificate code "+FormatCode(d.Code), "Courier", "B", 10.5, navy)
	if d.VerifyURL != "" {
		center(fy+24, 5, "Verify this certificate at "+d.VerifyURL, "Helvetica", "", 9, grey)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
