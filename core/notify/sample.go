package notify

import "github.com/vishnubalachandran596/portfolio/core/models"

// SampleData is realistic placeholder data for preview, send-test, and
// the save-time render check. It is not a customer record.
func SampleData(kind string) models.EmailData {
	d := models.EmailData{
		BuyerName:      "Asha Raman",
		ItemTitle:      "Sample course",
		Amount:         models.FormatMoney(149900, "INR"),
		ReceiptNo:      "CRS-1001",
		ReceiptURL:     "https://www.vishnubalachandran.com/account/purchases/1001/receipt.pdf",
		AccessURL:      "https://www.vishnubalachandran.com/courses/sample/learn",
		ClaimCode:      "AB12C-D34EF-56AB7-89012",
		SiteName:       "Vishnu Balachandran",
		SupportReplyTo: ReplyTo,
		BookingStart:   "Mon, Jan 2 2026 · 3:04 PM IST",
		BookingCode:    "BK-SAMPLE",
		Duration:       "60 minutes",
		ManageURL:      "https://www.vishnubalachandran.com/booking/manage/sample-token",
		UnsubscribeURL: "https://www.vishnubalachandran.com/contact",
	}
	switch kind {
	case models.EmailPurchaseSoftware:
		d.ItemTitle = "Sample software"
		d.ReceiptNo = "SW-1001"
		d.ReceiptURL = "https://www.vishnubalachandran.com/account/software/1001/receipt.pdf"
		d.AccessURL = "https://www.vishnubalachandran.com/account/licences"
		d.Instructions = "Open My licences, enter the Device ID from the app, and save the key."
		d.ClaimCode = ""
	case models.EmailPurchaseArticle:
		d.ItemTitle = "Sample article"
		d.ReceiptNo = "ART-1001"
		d.ReceiptURL = "https://www.vishnubalachandran.com/account/articles/1001/receipt.pdf"
		d.AccessURL = "https://www.vishnubalachandran.com/articles/sample"
		d.ClaimCode = ""
	case models.EmailContribution:
		d.ItemTitle = "Sample course"
		d.ReceiptNo = "1001"
		d.ReceiptURL = "https://www.vishnubalachandran.com/courses/sample"
		d.AccessURL = "https://www.vishnubalachandran.com/courses/sample"
		d.ClaimCode = ""
	case models.EmailConnectRequest:
		d.BuyerName = "Asha Raman"
		d.ItemTitle = ""
		d.Amount = ""
		d.ManageURL = "https://www.vishnubalachandran.com/booking"
	case models.EmailNewsletterConfirm:
		d.BuyerName = "there"
		d.ItemTitle = ""
	}
	return d
}
