package dto

import (
	"net/netip"
	"time"

	"github.com/google/uuid"
)

// Role is a user role.
type Role string

// Enum lists the six roles for OpenAPI.
func (Role) Enum() []any {
	return []any{"admin", "company_admin", "company_readonly_admin", "building_admin", "building_readonly_admin", "demo"}
}

// Permission is one R159 permission name.
type Permission string

// Enum lists every permission for OpenAPI; kept equal to auth.AllPermissions by test.
func (Permission) Enum() []any {
	return []any{
		"admin.companies", "alarms.edit", "alarms.evaluate", "alarms.read", "analyzers.refresh",
		"anomaly.check", "bills.compute", "bills.read", "calendar.edit", "carbon.edit", "carbon.read", "financial.read", "forecast.read", "forecast.run", "integrations.credentials", "iso50001.edit", "iso50001.read",
		"jobs.runs.read", "jobs.trigger", "messages.read", "nav.core", "nav.financial", "nav.solar_plants",
		"plants.manage", "plants.read", "renewable.read", "reports.email", "reports.generate", "reports.read", "settings.analyzers", "settings.analyzers.edit", "settings.buildings", "settings.company",
		"settings.company.edit", "settings.integrations", "settings.plants", "settings.smtp",
		"settings.users", "solar_tariffs.read", "tariffs.bulk.read", "tariffs.defaults", "tariffs.edit",
		"tariffs.icmal", "tariffs.read", "tariffs.templates.read", "write",
	}
}

// Locale is tr or en.
type Locale string

// Enum lists the locales.
func (Locale) Enum() []any { return []any{"tr", "en"} }

// LoginRequest is POST /auth/login.
type LoginRequest struct {
	Email      string `json:"email" validate:"required,email,max=254" required:"true" format:"email"`
	Password   string `json:"password" validate:"required,max=256" required:"true"`
	RememberMe bool   `json:"remember_me"`
}

// MobileLoginRequest is POST /mobile/auth/login.
type MobileLoginRequest struct {
	Email    string `json:"email" validate:"required,email,max=254" required:"true" format:"email"`
	Password string `json:"password" validate:"required,max=256" required:"true"`
}

// MobileRefreshRequest is POST /mobile/auth/refresh.
type MobileRefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required,max=128" required:"true"`
}

// CompanyRef names a company.
type CompanyRef struct {
	ID   uuid.UUID `json:"id" required:"true"`
	Name string    `json:"name" required:"true"`
}

// Me is the current principal (05 §2 GET /auth/me).
type Me struct {
	ID            uuid.UUID    `json:"id" required:"true"`
	Name          string       `json:"name" required:"true"`
	Email         string       `json:"email" required:"true"`
	Phone         *string      `json:"phone"`
	Role          Role         `json:"role" required:"true"`
	Locale        Locale       `json:"locale" required:"true"`
	UIPreferences *string      `json:"ui_preferences"`
	Company       CompanyRef   `json:"company" required:"true"`
	Permissions   []Permission `json:"permissions" required:"true"`
	SessionID     uuid.UUID    `json:"session_id" required:"true"`
}

// MobileTokens is the mobile login and refresh response (05 §18).
type MobileTokens struct {
	AccessToken  string `json:"access_token" required:"true"`
	RefreshToken string `json:"refresh_token" required:"true"`
	TokenType    string `json:"token_type" required:"true" enum:"Bearer"`
	ExpiresIn    int64  `json:"expires_in" required:"true"`
	User         Me     `json:"user" required:"true"`
}

// ForgotPasswordRequest is POST /auth/forgot-password.
type ForgotPasswordRequest struct {
	Email string `json:"email" validate:"required,email,max=254" required:"true" format:"email"`
}

// ResetPasswordRequest is POST /auth/reset-password.
type ResetPasswordRequest struct {
	Token    string `json:"token" validate:"required,max=128" required:"true"`
	Password string `json:"password" validate:"required,max=256" required:"true"`
}

// ChangePasswordRequest is POST /auth/change-password.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" validate:"required,max=256" required:"true"`
	NewPassword     string `json:"new_password" validate:"required,max=256" required:"true"`
}

// Session is one active session (GET /auth/sessions).
type Session struct {
	ID         uuid.UUID   `json:"id" required:"true"`
	Client     string      `json:"client" required:"true" enum:"web,mobile"`
	UserAgent  *string     `json:"user_agent"`
	IP         *netip.Addr `json:"ip"`
	CreatedAt  time.Time   `json:"created_at" required:"true"`
	LastUsedAt *time.Time  `json:"last_used_at"`
	ExpiresAt  time.Time   `json:"expires_at" required:"true"`
	Current    bool        `json:"current" required:"true"`
}

// SessionList is GET /auth/sessions.
type SessionList struct {
	Items []Session `json:"items" required:"true"`
}

// IDPath is a route's {id}.
type IDPath struct {
	ID uuid.UUID `path:"id" json:"-"`
}

// ProfileUpdateRequest is PATCH /profile; omitted fields are kept.
type ProfileUpdateRequest struct {
	Name          *string `json:"name,omitempty" validate:"omitempty,min=1,max=200"`
	Email         *string `json:"email,omitempty" validate:"omitempty,email,max=254" format:"email"`
	Phone         *string `json:"phone,omitempty" validate:"omitempty,max=40"`
	Locale        *Locale `json:"locale,omitempty" validate:"omitempty,oneof=tr en"`
	UIPreferences *string `json:"ui_preferences,omitempty" validate:"omitempty,max=512"`
}
