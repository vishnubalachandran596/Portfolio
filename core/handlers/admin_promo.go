package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/vishnubalachandran596/portfolio/core/models"
	"github.com/vishnubalachandran596/portfolio/core/repo"
)

// promoDateTimeLayout matches the value format an HTML
// <input type="datetime-local"> submits and expects back.
const promoDateTimeLayout = "2006-01-02T15:04"

type AdminPromoListData struct {
	BaseData
	CoursePromos      []models.PromoCode
	ConsultancyPromos []models.PromoCode
}

func (a *App) AdminPromoList(w http.ResponseWriter, r *http.Request) {
	all, err := a.PromoCodes.ListAll(r.Context())
	if err != nil {
		a.serverError(w, "load promo codes", err)
		return
	}
	data := AdminPromoListData{BaseData: a.base(r, "Promo Codes — Admin", "", "admin")}
	for _, p := range all {
		if p.Product == models.PromoProductConsultancy {
			data.ConsultancyPromos = append(data.ConsultancyPromos, p)
		} else {
			data.CoursePromos = append(data.CoursePromos, p)
		}
	}
	a.View.Render(w, http.StatusOK, "admin_promo_list", data)
}

type AdminPromoFormData struct {
	BaseData
	Promo             models.PromoCode
	StartsAtValue     string
	ExpiresAtValue    string
	AllowedEmailsText string
	Courses           []models.Course
	IsNew             bool
	Error             string
}

func (a *App) AdminPromoNewPage(w http.ResponseWriter, r *http.Request) {
	courses, err := a.Courses.ListAll(r.Context())
	if err != nil {
		a.serverError(w, "load courses", err)
		return
	}
	product := models.PromoProductCourse
	if r.URL.Query().Get("product") == models.PromoProductConsultancy {
		product = models.PromoProductConsultancy
	}
	a.View.Render(w, http.StatusOK, "admin_promo_form", AdminPromoFormData{
		BaseData: a.base(r, "New Promo Code — Admin", "", "admin"),
		Promo:    models.PromoCode{Product: product, IsActive: true, MaxRedemptionsPerUser: 1},
		Courses:  courses,
		IsNew:    true,
	})
}

func (a *App) AdminPromoEditPage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	promo, err := a.PromoCodes.GetByID(r.Context(), id)
	if err != nil {
		if err == repo.ErrNotFound {
			a.NotFound(w, r)
			return
		}
		a.serverError(w, "load promo code", err)
		return
	}
	courses, err := a.Courses.ListAll(r.Context())
	if err != nil {
		a.serverError(w, "load courses", err)
		return
	}
	a.View.Render(w, http.StatusOK, "admin_promo_form", AdminPromoFormData{
		BaseData:          a.base(r, "Edit Promo Code — Admin", "", "admin"),
		Promo:             promo,
		StartsAtValue:     promo.StartsAt.In(a.Timezone).Format(promoDateTimeLayout),
		ExpiresAtValue:    promo.ExpiresAt.In(a.Timezone).Format(promoDateTimeLayout),
		AllowedEmailsText: strings.Join(promo.AllowedEmails, ", "),
		Courses:           courses,
	})
}

func (a *App) AdminPromoSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	var id int64
	if idStr := r.PathValue("id"); idStr != "" {
		var err error
		id, err = strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			a.NotFound(w, r)
			return
		}
	}

	product := models.PromoProductCourse
	if r.FormValue("product") == models.PromoProductConsultancy {
		product = models.PromoProductConsultancy
	}
	discountPercent, _ := strconv.Atoi(r.FormValue("discount_percent"))
	maxRedemptions, _ := strconv.Atoi(r.FormValue("max_redemptions"))
	maxRedemptionsPerUser, _ := strconv.Atoi(r.FormValue("max_redemptions_per_user"))
	startsAtRaw := strings.TrimSpace(r.FormValue("starts_at"))
	startsAt, startErr := time.ParseInLocation(promoDateTimeLayout, startsAtRaw, a.Timezone)
	expiresAtRaw := strings.TrimSpace(r.FormValue("expires_at"))
	expiresAt, expErr := time.ParseInLocation(promoDateTimeLayout, expiresAtRaw, a.Timezone)

	var courseIDs []int64
	if product == models.PromoProductCourse {
		for _, raw := range r.Form["course_ids"] {
			if cid, err := strconv.ParseInt(raw, 10, 64); err == nil {
				courseIDs = append(courseIDs, cid)
			}
		}
	}

	allowedEmailsRaw := strings.TrimSpace(r.FormValue("allowed_emails"))
	var allowedEmails []string
	for _, e := range strings.Split(allowedEmailsRaw, ",") {
		e = strings.ToLower(strings.TrimSpace(e))
		if e != "" {
			allowedEmails = append(allowedEmails, e)
		}
	}
	// A user-scoped code is always private — the checkbox is ignored (and
	// disabled client-side, see the template) once any email is listed.
	isPublic := r.FormValue("is_public") == "on" && len(allowedEmails) == 0

	promo := models.PromoCode{
		ID:                    id,
		Code:                  strings.ToUpper(strings.TrimSpace(r.FormValue("code"))),
		Description:           strings.TrimSpace(r.FormValue("description")),
		BannerText:            strings.TrimSpace(r.FormValue("banner_text")),
		Product:               product,
		DiscountPercent:       discountPercent,
		CourseIDs:             courseIDs,
		MaxRedemptions:        maxRedemptions,
		MaxRedemptionsPerUser: maxRedemptionsPerUser,
		StartsAt:              startsAt,
		ExpiresAt:             expiresAt,
		IsActive:              r.FormValue("is_active") == "on",
		IsPublic:              isPublic,
		AllowedEmails:         allowedEmails,
	}

	formErr := ""
	switch {
	case promo.Code == "":
		formErr = "A code is required."
	case discountPercent <= 0 || discountPercent > 100:
		formErr = "Discount must be between 1 and 100 percent."
	case startErr != nil:
		formErr = "A valid start date and time is required."
	case expErr != nil:
		formErr = "A valid expiry date and time is required."
	case !expiresAt.After(startsAt):
		formErr = "The expiry must be after the start time."
	case maxRedemptions < 0:
		formErr = "Redemption limit can't be negative."
	case maxRedemptionsPerUser < 0:
		formErr = "Per-account redemption limit can't be negative."
	}
	if formErr != "" {
		courses, _ := a.Courses.ListAll(r.Context())
		a.View.Render(w, http.StatusUnprocessableEntity, "admin_promo_form", AdminPromoFormData{
			BaseData:          a.base(r, "Promo Code — Admin", "", "admin"),
			Promo:             promo,
			StartsAtValue:     startsAtRaw,
			ExpiresAtValue:    expiresAtRaw,
			AllowedEmailsText: allowedEmailsRaw,
			Courses:           courses,
			IsNew:             id == 0,
			Error:             formErr,
		})
		return
	}

	ctx := r.Context()
	var err error
	if id == 0 {
		_, err = a.PromoCodes.Create(ctx, promo)
	} else {
		err = a.PromoCodes.Update(ctx, promo)
	}
	if err != nil {
		if err == repo.ErrDuplicateCode {
			courses, _ := a.Courses.ListAll(ctx)
			a.View.Render(w, http.StatusUnprocessableEntity, "admin_promo_form", AdminPromoFormData{
				BaseData:          a.base(r, "Promo Code — Admin", "", "admin"),
				Promo:             promo,
				StartsAtValue:     startsAtRaw,
				ExpiresAtValue:    expiresAtRaw,
				AllowedEmailsText: allowedEmailsRaw,
				Courses:           courses,
				IsNew:             id == 0,
				Error:             "That code is already in use.",
			})
			return
		}
		a.serverError(w, "save promo code", err)
		return
	}
	http.Redirect(w, r, "/admin/promo-codes", http.StatusSeeOther)
}

func (a *App) AdminPromoDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.NotFound(w, r)
		return
	}
	if err := a.PromoCodes.Delete(r.Context(), id); err != nil {
		a.serverError(w, "delete promo code", err)
		return
	}
	http.Redirect(w, r, "/admin/promo-codes", http.StatusSeeOther)
}
