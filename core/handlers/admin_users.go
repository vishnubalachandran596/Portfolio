package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
)

type adminUserRow struct {
	models.User
	IsAdmin bool
}

type AdminUsersListData struct {
	BaseData
	Users []adminUserRow
}

// AdminUsersList shows every registered visitor account, flagging which
// ones are also admins.
func (a *App) AdminUsersList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	users, err := a.Users.ListAll(ctx)
	if err != nil {
		a.serverError(w, "load users", err)
		return
	}
	admins, err := a.Admins.ListAll(ctx)
	if err != nil {
		a.serverError(w, "load admins", err)
		return
	}
	adminEmails := map[string]bool{}
	for _, adm := range admins {
		adminEmails[adm.Email] = true
	}

	rows := make([]adminUserRow, 0, len(users))
	for _, u := range users {
		rows = append(rows, adminUserRow{User: u, IsAdmin: adminEmails[u.Email]})
	}

	a.View.Render(w, http.StatusOK, "admin_users_list", AdminUsersListData{
		BaseData: a.base(r, "Users — Admin", "", "admin"),
		Users:    rows,
	})
}

type adminUserBooking struct {
	When   string
	Status string
}

type adminUserPurchase struct {
	CourseTitle string
	Amount      string
	Status      string
	When        string
}

type adminUserMessage struct {
	When string
	Body string
}

type adminUserContribution struct {
	ArticleTitle string
	ArticleID    int64
	Amount       string
	Status       string
	When         string
}

type AdminUserDetailData struct {
	BaseData
	User          models.User
	IsAdmin       bool
	CanDemote     bool
	Bookings      []adminUserBooking
	Purchases     []adminUserPurchase
	Contributions []adminUserContribution
	Messages      []adminUserMessage
}

// AdminUserDetail shows one visitor's bookings and course purchases, plus
// the promote/demote-to-admin control.
func (a *App) AdminUserDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	user, err := a.Users.GetByID(ctx, id)
	if err != nil {
		if err == repo.ErrNotFound {
			a.NotFound(w, r)
			return
		}
		a.serverError(w, "load user", err)
		return
	}

	_, aerr := a.Admins.GetByEmail(ctx, user.Email)
	isAdmin := aerr == nil

	adminCount := 0
	if isAdmin {
		if all, err := a.Admins.ListAll(ctx); err == nil {
			adminCount = len(all)
		}
	}

	bookings, err := a.Bookings.ListByUserID(ctx, id)
	if err != nil {
		a.serverError(w, "load bookings", err)
		return
	}
	purchases, err := a.Purchases.ListByUserID(ctx, id)
	if err != nil {
		a.serverError(w, "load purchases", err)
		return
	}
	messages, err := a.AdminMessages.ListByUserID(ctx, id)
	if err != nil {
		a.serverError(w, "load messages", err)
		return
	}
	contribs, err := a.ArticleContributions.ListByUserID(ctx, id)
	if err != nil {
		a.serverError(w, "load article contributions", err)
		return
	}

	data := AdminUserDetailData{
		BaseData:  a.base(r, user.Name+" — Admin", "", "admin"),
		User:      user,
		IsAdmin:   isAdmin,
		CanDemote: isAdmin && adminCount > 1,
	}
	for _, b := range bookings {
		data.Bookings = append(data.Bookings, adminUserBooking{
			When:   b.SlotStart.In(a.Timezone).Format("Mon, Jan 2 2006 · 3:04 PM"),
			Status: b.Status,
		})
	}
	for _, p := range purchases {
		course, cerr := a.Courses.GetByID(ctx, p.CourseID)
		title := "(course removed)"
		if cerr == nil {
			title = course.Title
		}
		when := p.CreatedAt.In(a.Timezone).Format("Jan 2, 2006")
		if p.PaidAt != nil {
			when = p.PaidAt.In(a.Timezone).Format("Jan 2, 2006")
		}
		data.Purchases = append(data.Purchases, adminUserPurchase{
			CourseTitle: title,
			Amount:      models.FormatMoney(p.AmountCents, p.Currency),
			Status:      p.Status,
			When:        when,
		})
	}
	for _, m := range messages {
		data.Messages = append(data.Messages, adminUserMessage{
			When: m.CreatedAt.In(a.Timezone).Format("Mon, Jan 2 2006 · 3:04 PM"),
			Body: m.Body,
		})
	}
	for _, c := range contribs {
		article, aerr := a.Articles.GetByID(ctx, c.ArticleID)
		title := "(article removed)"
		if aerr == nil {
			title = article.Title
		}
		when := c.CreatedAt.In(a.Timezone).Format("Jan 2, 2006 · 3:04 PM")
		if c.PaidAt != nil {
			when = c.PaidAt.In(a.Timezone).Format("Jan 2, 2006 · 3:04 PM")
		}
		data.Contributions = append(data.Contributions, adminUserContribution{
			ArticleTitle: title,
			ArticleID:    c.ArticleID,
			Amount:       models.FormatMoney(c.AmountCents, c.Currency),
			Status:       c.Status,
			When:         when,
		})
	}

	a.View.Render(w, http.StatusOK, "admin_user_detail", data)
}

// AdminUserMessageSend sends a message to a visitor's account, always
// closing with a fixed signature pointing them at the admin's own email
// — never editable per-message, so every message reads consistently.
func (a *App) AdminUserMessageSend(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	if _, err := a.Users.GetByID(ctx, id); err != nil {
		if err == repo.ErrNotFound {
			a.NotFound(w, r)
			return
		}
		a.serverError(w, "load user", err)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	body := strings.TrimSpace(r.FormValue("body"))
	if body == "" {
		http.Redirect(w, r, "/admin/users/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
		return
	}

	body = a.withContactSignature(ctx, body)

	if _, err := a.AdminMessages.Create(ctx, id, body); err != nil {
		a.serverError(w, "send message", err)
		return
	}
	http.Redirect(w, r, "/admin/users/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// AdminUserPromote makes a visitor account an admin too. Since they may
// have only ever signed in with Google, a random (never-usable) password
// hash is stored — same pattern as auto-provisioning a Google user —
// so they keep signing in the same way and it just also opens /admin.
func (a *App) AdminUserPromote(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	user, err := a.Users.GetByID(ctx, id)
	if err != nil {
		if err == repo.ErrNotFound {
			a.NotFound(w, r)
			return
		}
		a.serverError(w, "load user", err)
		return
	}

	if _, err := a.Admins.GetByEmail(ctx, user.Email); err == nil {
		http.Redirect(w, r, "/admin/users/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
		return
	}

	if _, err := a.Admins.Create(ctx, user.Email, user.Name); err != nil {
		a.serverError(w, "promote user", err)
		return
	}

	http.Redirect(w, r, "/admin/users/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// AdminUserDemote removes admin access from a visitor account. Refuses to
// remove the last remaining admin so the site can never be left without
// one.
func (a *App) AdminUserDemote(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	user, err := a.Users.GetByID(ctx, id)
	if err != nil {
		if err == repo.ErrNotFound {
			a.NotFound(w, r)
			return
		}
		a.serverError(w, "load user", err)
		return
	}

	admin, err := a.Admins.GetByEmail(ctx, user.Email)
	if err != nil {
		http.Redirect(w, r, "/admin/users/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
		return
	}
	// The configured owner email is re-added on every start, so removing it
	// here would not stick; refuse instead of pretending.
	if a.Cfg.AdminEmail != "" && strings.EqualFold(admin.Email, a.Cfg.AdminEmail) {
		http.Redirect(w, r, "/admin/users/"+strconv.FormatInt(id, 10)+"?error=The+owner+admin+email+is+set+in+ADMIN_BOOTSTRAP_EMAIL+and+can%27t+be+removed+here", http.StatusSeeOther)
		return
	}
	all, err := a.Admins.ListAll(ctx)
	if err != nil {
		a.serverError(w, "load admins", err)
		return
	}
	if len(all) <= 1 {
		http.Redirect(w, r, "/admin/users/"+strconv.FormatInt(id, 10)+"?error=Can%27t+remove+the+last+admin", http.StatusSeeOther)
		return
	}
	if err := a.Admins.Delete(ctx, admin.ID); err != nil {
		a.serverError(w, "demote user", err)
		return
	}

	http.Redirect(w, r, "/admin/users/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}
