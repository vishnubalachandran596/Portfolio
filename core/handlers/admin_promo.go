package handlers

import (
	"context"
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
	SoftwarePromos    []models.PromoCode
}

func (a *App) AdminPromoList(w http.ResponseWriter, r *http.Request) {
	all, err := a.PromoCodes.ListAll(r.Context())
	if err != nil {
		a.serverError(w, "load promo codes", err)
		return
	}
	data := AdminPromoListData{BaseData: a.base(r, "Promo Codes — Admin", "", "admin")}
	for _, p := range all {
		switch p.Product {
		case models.PromoProductConsultancy:
			data.ConsultancyPromos = append(data.ConsultancyPromos, p)
		case models.PromoProductSoftware:
			data.SoftwarePromos = append(data.SoftwarePromos, p)
		default:
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
	SoftwareItems     []models.Software
	IsNew             bool
	Error             string
}

// promoScopeChoices loads the course and software lists the form's two
// scope pickers offer.
func (a *App) promoScopeChoices(ctx context.Context) ([]models.Course, []models.Software, error) {
	courses, err := a.Courses.ListAll(ctx)
	if err != nil {
		return nil, nil, err
	}
	software, err := a.Software.ListAll(ctx)
	return courses, software, err
}

func (a *App) AdminPromoNewPage(w http.ResponseWriter, r *http.Request) {
	courses, software, err := a.promoScopeChoices(r.Context())
	if err != nil {
		a.serverError(w, "load promo scope choices", err)
		return
	}
	product := models.PromoProductCourse
	switch r.URL.Query().Get("product") {
	case models.PromoProductConsultancy:
		product = models.PromoProductConsultancy
	case models.PromoProductSoftware:
		product = models.PromoProductSoftware
	}
	a.View.Render(w, http.StatusOK, "admin_promo_form", AdminPromoFormData{
		BaseData:      a.base(r, "New Promo Code — Admin", "", "admin"),
		Promo:         models.PromoCode{Product: product, IsActive: true, MaxRedemptionsPerUser: 1},
		Courses:       courses,
		SoftwareItems: software,
		IsNew:         true,
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
	courses, software, err := a.promoScopeChoices(r.Context())
	if err != nil {
		a.serverError(w, "load promo scope choices", err)
		return
	}
	a.View.Render(w, http.StatusOK, "admin_promo_form", AdminPromoFormData{
		BaseData:          a.base(r, "Edit Promo Code — Admin", "", "admin"),
		Promo:             promo,
		StartsAtValue:     promo.StartsAt.In(a.Timezone).Format(promoDateTimeLayout),
		ExpiresAtValue:    promo.ExpiresAt.In(a.Timezone).Format(promoDateTimeLayout),
		AllowedEmailsText: strings.Join(promo.AllowedEmails, ", "),
		Courses:           courses,
		SoftwareItems:     software,
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
	switch r.FormValue("product") {
	case models.PromoProductConsultancy:
		product = models.PromoProductConsultancy
	case models.PromoProductSoftware:
		product = models.PromoProductSoftware
	}
	discountPercent, _ := strconv.Atoi(r.FormValue("discount_percent"))
	maxRedemptions, _ := strconv.Atoi(r.FormValue("max_redemptions"))
	maxRedemptionsPerUser, _ := strconv.Atoi(r.FormValue("max_redemptions_per_user"))
	startsAtRaw := strings.TrimSpace(r.FormValue("starts_at"))
	startsAt, startErr := time.ParseInLocation(promoDateTimeLayout, startsAtRaw, a.Timezone)
	expiresAtRaw := strings.TrimSpace(r.FormValue("expires_at"))
	expiresAt, expErr := time.ParseInLocation(promoDateTimeLayout, expiresAtRaw, a.Timezone)

	// Only the scope matching the selected product is kept, so switching a
	// code's product never leaves a stale scope behind.
	var courseIDs, softwareIDs []int64
	switch product {
	case models.PromoProductCourse:
		courseIDs = parseIDList(r.Form["course_ids"])
	case models.PromoProductSoftware:
		softwareIDs = parseIDList(r.Form["software_ids"])
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
		SoftwareIDs:           softwareIDs,
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
		courses, software, _ := a.promoScopeChoices(r.Context())
		a.View.Render(w, http.StatusUnprocessableEntity, "admin_promo_form", AdminPromoFormData{
			BaseData:          a.base(r, "Promo Code — Admin", "", "admin"),
			Promo:             promo,
			StartsAtValue:     startsAtRaw,
			ExpiresAtValue:    expiresAtRaw,
			AllowedEmailsText: allowedEmailsRaw,
			Courses:           courses,
			SoftwareItems:     software,
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
			courses, software, _ := a.promoScopeChoices(ctx)
			a.View.Render(w, http.StatusUnprocessableEntity, "admin_promo_form", AdminPromoFormData{
				BaseData:          a.base(r, "Promo Code — Admin", "", "admin"),
				Promo:             promo,
				StartsAtValue:     startsAtRaw,
				ExpiresAtValue:    expiresAtRaw,
				AllowedEmailsText: allowedEmailsRaw,
				Courses:           courses,
				SoftwareItems:     software,
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

// parseIDList converts submitted checkbox values to IDs, skipping any that
// aren't numbers.
func parseIDList(raw []string) []int64 {
	var out []int64
	for _, v := range raw {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			out = append(out, id)
		}
	}
	return out
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
