package handlers

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/authutil"
	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/notify"
	"github.com/vishnubalachandran596/portfolio/core/receipt"
)

func (a *App) absURL(path string) string {
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	base := ""
	if a.Cfg != nil {
		base = strings.TrimRight(strings.TrimSpace(a.Cfg.SiteURL), "/")
	}
	if base == "" {
		return path
	}
	return base + path
}

func (a *App) siteName() string {
	if a.Cfg != nil && strings.TrimSpace(a.Cfg.SiteName) != "" {
		return a.Cfg.SiteName
	}
	return notify.DisplayName
}

func (a *App) emailWhen(t time.Time) string {
	loc := a.Timezone
	if loc == nil {
		loc = time.UTC
	}
	return t.In(loc).Format("Mon, Jan 2 2006 · 3:04 PM MST")
}

func (a *App) baseEmailData() models.EmailData {
	return models.EmailData{BuyerName: "there", SiteName: a.siteName(), SupportReplyTo: notify.ReplyTo}
}

// spammyName reports a name no real visitor types: over 60 characters, or
// containing a link, an address, angle brackets or a line break. The contact
// and newsletter forms are public, so a bot can type anything into the name
// box.
func spammyName(name string) bool {
	n := strings.TrimSpace(name)
	low := strings.ToLower(n)
	return len([]rune(n)) > 60 || strings.ContainsAny(n, "@\r\n<>") ||
		strings.Contains(low, "http") || strings.Contains(low, "www.")
}

// buyerName is the name echoed into an outgoing email; a spammy or empty name
// is never echoed into mail from this domain.
func buyerName(name, email string) string {
	if n := strings.TrimSpace(name); n != "" && !spammyName(n) {
		return n
	}
	return "there"
}

func (a *App) bookingManageURL(token string, userID int64) string {
	if token != "" {
		return a.absURL("/booking/manage/" + url.PathEscape(token))
	}
	if userID > 0 {
		return a.absURL("/account")
	}
	return a.absURL("/booking")
}

func (a *App) emailCoursePurchase(ctx context.Context, p models.Purchase, course models.Course) {
	if p.ID == 0 {
		return
	}
	a.dispatchEmail(ctx, a.courseEmailJob(p, course))
}

func (a *App) courseEmailJob(p models.Purchase, course models.Course) emailJob {
	d := a.baseEmailData()
	d.BuyerName = buyerName("", p.Email)
	d.ItemTitle = course.Title
	d.Amount = models.FormatMoney(p.AmountCents, p.Currency)
	d.ReceiptNo = receipt.ReceiptNoForPurchase(p.ID)
	if p.UserID != 0 {
		d.ReceiptURL = a.absURL("/account/purchases/" + strconv.FormatInt(p.ID, 10) + "/receipt.pdf")
	} else {
		d.ReceiptURL = a.absURL("/account/claim")
	}
	d.AccessURL = a.absURL("/courses/" + course.Slug + "/learn")
	if p.ClaimCode != "" {
		d.ClaimCode = authutil.FormatClaimCode(p.ClaimCode)
	}
	return emailJob{Kind: models.EmailPurchaseCourse, EntityID: strconv.FormatInt(p.ID, 10), To: p.Email, Data: d}
}

func (a *App) emailSoftwarePurchase(ctx context.Context, p models.SoftwarePurchase, sw models.Software) {
	if p.ID == 0 {
		return
	}
	a.dispatchEmail(ctx, a.softwareEmailJob(p, sw))
}

func (a *App) softwareEmailJob(p models.SoftwarePurchase, sw models.Software) emailJob {
	d := a.baseEmailData()
	d.BuyerName = buyerName("", p.Email)
	d.ItemTitle = strings.TrimSpace(sw.Title + " " + sw.Version)
	d.Amount = models.FormatMoney(p.AmountCents, p.Currency)
	d.ReceiptNo = receipt.ReceiptNoForSoftwarePurchase(p.ID)
	d.ReceiptURL = a.absURL("/account/software/" + strconv.FormatInt(p.ID, 10) + "/receipt.pdf")
	d.AccessURL = a.absURL("/account/licences")
	d.Instructions = sw.PostPurchaseInstructions
	return emailJob{Kind: models.EmailPurchaseSoftware, EntityID: strconv.FormatInt(p.ID, 10), To: p.Email, Data: d}
}

func (a *App) emailArticleContribution(ctx context.Context, c models.ArticleContribution, article models.Article) {
	if c.ID == 0 {
		return
	}
	a.dispatchEmail(ctx, a.articleEmailJob(c, article))
}

func (a *App) articleEmailJob(c models.ArticleContribution, article models.Article) emailJob {
	d := a.baseEmailData()
	d.BuyerName = buyerName("", c.Email)
	d.ItemTitle = article.Title
	d.Amount = models.FormatMoney(c.AmountCents, c.Currency)
	d.ReceiptNo = receipt.ReceiptNoForArticleContribution(c.ID)
	if c.UserID != 0 {
		d.ReceiptURL = a.absURL("/account/articles/" + strconv.FormatInt(c.ID, 10) + "/receipt.pdf")
	} else {
		d.ReceiptURL = a.absURL("/articles/" + article.Slug)
	}
	d.AccessURL = a.absURL("/articles/" + article.Slug)
	return emailJob{Kind: models.EmailPurchaseArticle, EntityID: strconv.FormatInt(c.ID, 10), To: c.Email, Data: d}
}

func (a *App) emailCourseContribution(ctx context.Context, c models.CourseContribution, course models.Course) {
	if c.ID == 0 {
		return
	}
	a.dispatchEmail(ctx, a.contributionEmailJob(c, course))
}

func (a *App) contributionEmailJob(c models.CourseContribution, course models.Course) emailJob {
	d := a.baseEmailData()
	d.BuyerName = buyerName("", c.Email)
	d.ItemTitle = course.Title
	d.Amount = models.FormatMoney(c.AmountCents, c.Currency)
	d.ReceiptNo = strconv.FormatInt(c.ID, 10)
	d.ReceiptURL = a.absURL("/courses/" + course.Slug)
	d.AccessURL = a.absURL("/courses/" + course.Slug)
	return emailJob{Kind: models.EmailContribution, EntityID: strconv.FormatInt(c.ID, 10), To: c.Email, Data: d}
}

func (a *App) emailBooking(ctx context.Context, kind string, b models.Booking, start, end time.Time, token string) {
	if b.ID == 0 {
		return
	}
	job := a.bookingEmailJob(kind, b, start, end, token)
	if a.dispatchEmail(ctx, job) == "duplicate" && kind == models.EmailBookingRescheduled && a.Outbox != nil {
		if ok, err := a.Outbox.Requeue(ctx, job.Kind, job.EntityID); err == nil && ok {
			a.sendQueued(ctx, job)
		}
	}
}

func (a *App) bookingEmailJob(kind string, b models.Booking, start, end time.Time, token string) emailJob {
	entity := strconv.FormatInt(b.ID, 10)
	mins := int(end.Sub(start).Minutes())
	if mins <= 0 {
		mins = models.ConsultancyDurationMinutes
	}
	d := a.baseEmailData()
	d.BuyerName = buyerName(b.Name, b.Email)
	d.ItemTitle = "Consultancy call"
	d.BookingStart = a.emailWhen(start)
	d.BookingCode = b.Code
	d.Duration = fmt.Sprintf("%d minutes", mins)
	d.ManageURL = a.bookingManageURL(token, b.UserID)
	return emailJob{Kind: kind, EntityID: entity, To: b.Email, Data: d}
}

func (a *App) emailConnectRequest(ctx context.Context, m models.ContactMessage) {
	if m.ID == 0 {
		return
	}
	a.dispatchEmail(ctx, a.connectEmailJob(m))
}

func (a *App) connectEmailJob(m models.ContactMessage) emailJob {
	d := a.baseEmailData()
	d.BuyerName = buyerName(m.Name, m.Email)
	d.ManageURL = a.absURL("/booking")
	return emailJob{Kind: models.EmailConnectRequest, EntityID: strconv.FormatInt(m.ID, 10), To: m.Email, Data: d}
}

func (a *App) emailNewsletterConfirm(ctx context.Context, id int64, email string) {
	if id == 0 {
		return
	}
	d := a.baseEmailData()
	d.UnsubscribeURL = a.absURL("/contact")
	a.dispatchEmail(ctx, emailJob{
		Kind:     models.EmailNewsletterConfirm,
		EntityID: strconv.FormatInt(id, 10),
		To:       email,
		Data:     d,
	})
}

func (a *App) rebuildEmail(ctx context.Context, kind, entity string) (emailJob, error) {
	switch kind {
	case models.EmailPurchaseCourse:
		id, err := strconv.ParseInt(entity, 10, 64)
		if err != nil || a.Purchases == nil || a.Courses == nil {
			return emailJob{}, fmt.Errorf("course purchase %s", entity)
		}
		p, err := a.Purchases.GetByID(ctx, id)
		if err != nil {
			return emailJob{}, err
		}
		course, err := a.Courses.GetByID(ctx, p.CourseID)
		if err != nil {
			return emailJob{}, err
		}
		return a.courseEmailJob(p, course), nil
	case models.EmailPurchaseSoftware:
		id, err := strconv.ParseInt(entity, 10, 64)
		if err != nil || a.SoftwarePurchases == nil || a.Software == nil {
			return emailJob{}, fmt.Errorf("software purchase %s", entity)
		}
		p, err := a.SoftwarePurchases.GetByID(ctx, id)
		if err != nil {
			return emailJob{}, err
		}
		sw, err := a.Software.GetByID(ctx, p.SoftwareID)
		if err != nil {
			return emailJob{}, err
		}
		return a.softwareEmailJob(p, sw), nil
	case models.EmailPurchaseArticle:
		id, err := strconv.ParseInt(entity, 10, 64)
		if err != nil || a.ArticleContributions == nil || a.Articles == nil {
			return emailJob{}, fmt.Errorf("article contribution %s", entity)
		}
		c, err := a.ArticleContributions.GetByID(ctx, id)
		if err != nil {
			return emailJob{}, err
		}
		article, err := a.Articles.GetByID(ctx, c.ArticleID)
		if err != nil {
			return emailJob{}, err
		}
		return a.articleEmailJob(c, article), nil
	case models.EmailContribution:
		id, err := strconv.ParseInt(entity, 10, 64)
		if err != nil || a.CourseContributions == nil || a.Courses == nil {
			return emailJob{}, fmt.Errorf("contribution %s", entity)
		}
		c, err := a.CourseContributions.GetByID(ctx, id)
		if err != nil {
			return emailJob{}, err
		}
		course, err := a.Courses.GetByID(ctx, c.CourseID)
		if err != nil {
			return emailJob{}, err
		}
		return a.contributionEmailJob(c, course), nil
	case models.EmailBookingNew, models.EmailBookingRescheduled, models.EmailBookingCancelled:
		if a.Bookings == nil {
			return emailJob{}, fmt.Errorf("booking %s", entity)
		}
		idPart, unixPart, hasUnix := strings.Cut(entity, ":")
		id, err := strconv.ParseInt(idPart, 10, 64)
		if err != nil {
			return emailJob{}, err
		}
		b, err := a.Bookings.GetByID(ctx, id)
		if err != nil {
			return emailJob{}, err
		}
		start, end := b.SlotStart, b.SlotEnd
		if kind == models.EmailBookingRescheduled && hasUnix {
			unix, uerr := strconv.ParseInt(unixPart, 10, 64)
			if uerr != nil {
				return emailJob{}, uerr
			}
			start = time.Unix(unix, 0)
			dur := b.SlotEnd.Sub(b.SlotStart)
			if dur <= 0 {
				dur = time.Duration(models.ConsultancyDurationMinutes) * time.Minute
			}
			end = start.Add(dur)
		}
		return a.bookingEmailJob(kind, b, start, end, ""), nil
	case models.EmailConnectRequest:
		id, err := strconv.ParseInt(entity, 10, 64)
		if err != nil || a.ContactMessages == nil {
			return emailJob{}, fmt.Errorf("connect request %s", entity)
		}
		m, err := a.ContactMessages.GetByID(ctx, id)
		if err != nil {
			return emailJob{}, err
		}
		return a.connectEmailJob(m), nil
	case models.EmailNewsletterConfirm:
		id, err := strconv.ParseInt(entity, 10, 64)
		if err != nil || a.Subscribers == nil {
			return emailJob{}, fmt.Errorf("subscriber %s", entity)
		}
		s, err := a.Subscribers.GetByID(ctx, id)
		if err != nil {
			return emailJob{}, err
		}
		d := a.baseEmailData()
		d.UnsubscribeURL = a.absURL("/contact")
		return emailJob{Kind: kind, EntityID: entity, To: s.Email, Data: d}, nil
	default:
		return emailJob{}, fmt.Errorf("cannot retry %s", kind)
	}
}
