// Package certificate renders a course-completion certificate as a
// landscape A4 PDF, generated in-process with a pure-Go PDF library (no
// headless browser or external service needed, so it works fine inside a
// Vercel Go serverless function).
package certificate

import (
	"bytes"
	"time"

	"github.com/go-pdf/fpdf"
)

type Data struct {
	RecipientName string
	CourseTitle   string
	IssuedAt      time.Time
	Code          string
	SiteName      string
	IssuerName    string
}

// Generate renders the certificate and returns the PDF bytes.
func Generate(d Data) ([]byte, error) {
	pdf := fpdf.New("L", "mm", "A4", "")
	pdf.SetMargins(0, 0, 0)
	pdf.SetAutoPageBreak(false, 0)
	pdf.AddPage()

	const pageW = 297.0

	pdf.SetDrawColor(180, 140, 60)
	pdf.SetLineWidth(1.2)
	pdf.Rect(8, 8, 281, 194, "D")
	pdf.SetLineWidth(0.4)
	pdf.Rect(11, 11, 275, 188, "D")

	center := func(y float64, h float64, txt string, font string, style string, size float64, r, g, b int) {
		pdf.SetFont(font, style, size)
		pdf.SetTextColor(r, g, b)
		pdf.SetY(y)
		pdf.CellFormat(pageW, h, txt, "", 1, "C", false, 0, "")
	}

	center(30, 14, "Certificate of Completion", "Helvetica", "B", 30, 40, 40, 40)
	center(55, 8, "This certifies that", "Helvetica", "", 14, 60, 60, 60)
	center(68, 14, d.RecipientName, "Helvetica", "B", 26, 120, 80, 20)
	center(90, 8, "has successfully completed the course", "Helvetica", "", 14, 60, 60, 60)
	center(101, 12, d.CourseTitle, "Helvetica", "B", 20, 40, 40, 40)
	center(150, 6, "Issued "+d.IssuedAt.Format("Jan 2, 2006")+"  ·  Certificate ID "+d.Code, "Helvetica", "", 11, 90, 90, 90)
	center(165, 6, d.IssuerName+" — "+d.SiteName, "Helvetica", "I", 12, 60, 60, 60)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
