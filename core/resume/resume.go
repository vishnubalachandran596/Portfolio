// Package resume renders a one-page-style PDF resume directly from the
// same profile/skills/experience/education data shown on the About page,
// generated in-process with a pure-Go PDF library (no headless browser or
// external service, so it works inside a Vercel Go serverless function).
// Because it's built from the live data, it never drifts out of sync with
// what the site itself says.
package resume

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/go-pdf/fpdf"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

type Data struct {
	Profile    models.Profile
	Skills     []models.SkillGroup
	Experience []models.Experience
	Education  []models.Education
}

const (
	marginL  = 16.0
	marginR  = 16.0
	pageW    = 210.0
	contentW = pageW - marginL - marginR
	inkR     = 30
	inkG     = 30
	inkB     = 30
	mutedR   = 100
	mutedG   = 100
	mutedB   = 100
	accentR  = 180
	accentG  = 60
	accentB  = 40
)

// Generate renders the resume and returns the PDF bytes.
func Generate(d Data) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(marginL, 16, marginR)
	pdf.SetAutoPageBreak(true, 16)
	pdf.AddPage()

	pdf.SetTextColor(inkR, inkG, inkB)
	pdf.SetFont("Helvetica", "B", 22)
	pdf.CellFormat(contentW, 10, d.Profile.Name, "", 1, "L", false, 0, "")

	pdf.SetFont("Helvetica", "", 12)
	pdf.SetTextColor(accentR, accentG, accentB)
	pdf.CellFormat(contentW, 7, d.Profile.Title, "", 1, "L", false, 0, "")

	var contactParts []string
	if d.Profile.Location != "" {
		contactParts = append(contactParts, d.Profile.Location)
	}
	if d.Profile.Email != "" {
		contactParts = append(contactParts, d.Profile.Email)
	}
	if d.Profile.LinkedInURL != "" {
		contactParts = append(contactParts, d.Profile.LinkedInURL)
	}
	if d.Profile.GitHubURL != "" {
		contactParts = append(contactParts, d.Profile.GitHubURL)
	}
	pdf.SetFont("Helvetica", "", 9.5)
	pdf.SetTextColor(mutedR, mutedG, mutedB)
	pdf.CellFormat(contentW, 6, strings.Join(contactParts, "  ·  "), "", 1, "L", false, 0, "")
	pdf.Ln(2)

	rule := func() {
		y := pdf.GetY()
		pdf.SetDrawColor(210, 210, 210)
		pdf.SetLineWidth(0.3)
		pdf.Line(marginL, y, pageW-marginR, y)
		pdf.Ln(4)
	}
	sectionHeading := func(title string) {
		rule()
		pdf.SetFont("Helvetica", "B", 11)
		pdf.SetTextColor(inkR, inkG, inkB)
		pdf.CellFormat(contentW, 6, strings.ToUpper(title), "", 1, "L", false, 0, "")
		pdf.Ln(1)
	}
	bodyText := func(txt string) {
		pdf.SetFont("Helvetica", "", 10)
		pdf.SetTextColor(60, 60, 60)
		pdf.MultiCell(contentW, 5.2, txt, "", "L", false)
	}

	if d.Profile.Summary != "" {
		sectionHeading("Summary")
		summary := d.Profile.Summary
		if i := strings.Index(summary, "\n\n"); i != -1 {
			summary = summary[:i]
		}
		bodyText(summary)
		pdf.Ln(2)
	}

	if len(d.Experience) > 0 {
		sectionHeading("Experience")
		for _, e := range d.Experience {
			pdf.SetFont("Helvetica", "B", 10.5)
			pdf.SetTextColor(inkR, inkG, inkB)
			pdf.CellFormat(contentW-40, 5.5, e.Role+" — "+e.Company, "", 0, "L", false, 0, "")
			pdf.SetFont("Helvetica", "", 9.5)
			pdf.SetTextColor(mutedR, mutedG, mutedB)
			pdf.CellFormat(40, 5.5, e.StartDate+" – "+e.EndDate, "", 1, "R", false, 0, "")
			if e.Location != "" {
				pdf.SetFont("Helvetica", "I", 9)
				pdf.SetTextColor(mutedR, mutedG, mutedB)
				pdf.CellFormat(contentW, 5, e.Location, "", 1, "L", false, 0, "")
			}
			pdf.SetFont("Helvetica", "", 9.5)
			pdf.SetTextColor(60, 60, 60)
			for _, b := range e.Bullets {
				pdf.SetX(marginL)
				pdf.MultiCell(contentW, 4.6, "•  "+b, "", "L", false)
			}
			pdf.Ln(2)
		}
	}

	if len(d.Skills) > 0 {
		sectionHeading("Skills")
		for _, s := range d.Skills {
			pdf.SetFont("Helvetica", "B", 9.5)
			pdf.SetTextColor(inkR, inkG, inkB)
			label := s.Category + ": "
			pdf.CellFormat(pdf.GetStringWidth(label)+2, 5, label, "", 0, "L", false, 0, "")
			pdf.SetFont("Helvetica", "", 9.5)
			pdf.SetTextColor(60, 60, 60)
			pdf.MultiCell(contentW-pdf.GetStringWidth(label)-2, 5, strings.Join(s.Items, ", "), "", "L", false)
		}
		pdf.Ln(1)
	}

	if len(d.Education) > 0 {
		sectionHeading("Education")
		for _, ed := range d.Education {
			pdf.SetFont("Helvetica", "B", 10)
			pdf.SetTextColor(inkR, inkG, inkB)
			degree := ed.Degree
			if ed.Field != "" {
				degree += ", " + ed.Field
			}
			pdf.CellFormat(contentW-30, 5.5, degree+" — "+ed.Institution, "", 0, "L", false, 0, "")
			pdf.SetFont("Helvetica", "", 9.5)
			pdf.SetTextColor(mutedR, mutedG, mutedB)
			years := ""
			if ed.StartYear > 0 || ed.EndYear > 0 {
				years = strconv.Itoa(ed.StartYear) + " – " + strconv.Itoa(ed.EndYear)
			}
			pdf.CellFormat(30, 5.5, years, "", 1, "R", false, 0, "")
		}
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
