// Package models defines the domain types persisted by the portfolio site.
package models

import (
	"fmt"
	"strconv"
	"time"
)

type Admin struct {
	ID           int64
	Email        string
	PasswordHash string
	Name         string
	CreatedAt    time.Time
}

type Profile struct {
	Name            string
	Title           string
	Tagline         string
	HeroSubtitle    string
	Summary         string
	Location        string
	Email           string
	LinkedInURL     string
	GitHubURL       string
	ResumeURL       string
	PhotoURL        string
	YearsExperience int
	UpdatedAt       time.Time
}

type SkillGroup struct {
	ID        int64
	Category  string
	Level     string
	Items     []string
	SortOrder int
}

type Experience struct {
	ID        int64
	Company   string
	Role      string
	Location  string
	StartDate string
	EndDate   string
	Bullets   []string
	SortOrder int
}

type Education struct {
	ID          int64
	Institution string
	Degree      string
	Field       string
	StartYear   int
	EndYear     int
	Details     string
	SortOrder   int
}

type HomeFeature struct {
	ID          int64
	Title       string
	Description string
	SortOrder   int
}

type Course struct {
	ID          int64
	Slug        string
	Title       string
	Summary     string
	Description string
	Level       string
	Duration    string
	PriceCents  int
	Currency    string
	ImageURL    string
	IsPublished bool
	SortOrder   int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// IsFree reports whether the course has no listed price.
func (c Course) IsFree() bool { return c.PriceCents == 0 }

// PriceDisplay renders the price as a human string, e.g. "$49" or "Free".
func (c Course) PriceDisplay() string {
	if c.IsFree() {
		return "Free"
	}
	symbol := "$"
	switch c.Currency {
	case "EUR":
		symbol = "€"
	case "GBP":
		symbol = "£"
	case "INR":
		symbol = "₹"
	}
	whole := c.PriceCents / 100
	cents := c.PriceCents % 100
	if cents == 0 {
		return symbol + strconv.Itoa(whole)
	}
	return symbol + strconv.Itoa(whole) + fmt.Sprintf(".%02d", cents)
}

const (
	BookingStatusPending   = "pending"
	BookingStatusConfirmed = "confirmed"
	BookingStatusCancelled = "cancelled"
)

type Booking struct {
	ID        int64
	Name      string
	Email     string
	Message   string
	SlotStart time.Time
	SlotEnd   time.Time
	Status    string
	CreatedAt time.Time
}

type AvailabilityRule struct {
	ID          int64
	Weekday     time.Weekday
	StartTime   string // "HH:MM"
	EndTime     string // "HH:MM"
	SlotMinutes int
	Active      bool
}

// TimeSlot is a bookable slot computed from availability rules minus
// existing bookings, shown to visitors on the booking page.
type TimeSlot struct {
	Start     time.Time
	End       time.Time
	Available bool
}
