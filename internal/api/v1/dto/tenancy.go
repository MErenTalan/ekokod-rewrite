package dto

import (
	"time"

	"github.com/google/uuid"
)

// Company is a tenant.
type Company struct {
	ID             uuid.UUID `json:"id" required:"true"`
	Name           string    `json:"name" required:"true"`
	Address        *string   `json:"address"`
	TotalAreaM2    *Decimal  `json:"total_area_m2"`
	PersonnelCount *int32    `json:"personnel_count"`
	ContactName    *string   `json:"contact_name"`
	ContactPhone   *string   `json:"contact_phone"`
	Sector         *string   `json:"sector"`
	CreatedAt      time.Time `json:"created_at" required:"true"`
	UpdatedAt      time.Time `json:"updated_at" required:"true"`
}

// AnalyzerCount is analyzers per provider and subtype.
type AnalyzerCount struct {
	Provider string `json:"provider" required:"true"`
	Subtype  string `json:"subtype" required:"true"`
	Count    int    `json:"count" required:"true"`
}

// CompanyDetail is GET /companies/{id}.
type CompanyDetail struct {
	Company
	AnalyzerCounts []AnalyzerCount `json:"analyzer_counts" required:"true"`
}

// CompanyListRequest is GET /companies.
type CompanyListRequest struct {
	Q      string  `query:"q" json:"-"`
	Sector *string `query:"sector" json:"-"`
	PageRequest
}

// CompanyFields are the writable company fields; omitted fields are kept, "" clears.
type CompanyFields struct {
	Address        *string  `json:"address,omitempty" validate:"omitempty,max=500"`
	TotalAreaM2    *Decimal `json:"total_area_m2,omitempty"`
	PersonnelCount *int32   `json:"personnel_count,omitempty" validate:"omitempty,min=0"`
	ContactName    *string  `json:"contact_name,omitempty" validate:"omitempty,max=200"`
	ContactPhone   *string  `json:"contact_phone,omitempty" validate:"omitempty,max=40"`
	Sector         *string  `json:"sector,omitempty" validate:"omitempty,max=100"`
}

// CompanyCreateRequest is POST /companies.
type CompanyCreateRequest struct {
	Name string `json:"name" validate:"required,max=200" required:"true"`
	CompanyFields
}

// CompanyUpdateRequest is PATCH /companies/{id}.
type CompanyUpdateRequest struct {
	ID   uuid.UUID `path:"id" json:"-"`
	Name *string   `json:"name,omitempty" validate:"omitempty,min=1,max=200"`
	CompanyFields
}

// User is a user as managers see them.
type User struct {
	ID          uuid.UUID  `json:"id" required:"true"`
	Name        string     `json:"name" required:"true"`
	Email       string     `json:"email" required:"true"`
	Phone       *string    `json:"phone"`
	Role        Role       `json:"role" required:"true"`
	IsActive    bool       `json:"is_active" required:"true"`
	Locale      Locale     `json:"locale" required:"true"`
	LastLoginAt *time.Time `json:"last_login_at"`
	CreatedAt   time.Time  `json:"created_at" required:"true"`
}

// UserListRequest is GET /users.
type UserListRequest struct {
	Role     []Role `query:"role" json:"-"`
	IsActive *bool  `query:"is_active" json:"-"`
	Q        string `query:"q" json:"-"`
	PageRequest
}

// UserCreateRequest is POST /users.
type UserCreateRequest struct {
	Name     string  `json:"name" validate:"required,max=200" required:"true"`
	Email    string  `json:"email" validate:"required,email,max=254" required:"true" format:"email"`
	Phone    *string `json:"phone,omitempty" validate:"omitempty,max=40"`
	Role     Role    `json:"role" validate:"required,oneof=admin company_admin company_readonly_admin building_admin building_readonly_admin demo" required:"true"`
	Password string  `json:"password" validate:"required,max=256" required:"true"`
	IsActive *bool   `json:"is_active,omitempty"`
}

// UserUpdateRequest is PATCH /users/{id}.
type UserUpdateRequest struct {
	ID       uuid.UUID `path:"id" json:"-"`
	Name     *string   `json:"name,omitempty" validate:"omitempty,min=1,max=200"`
	Email    *string   `json:"email,omitempty" validate:"omitempty,email,max=254" format:"email"`
	Phone    *string   `json:"phone,omitempty" validate:"omitempty,max=40"`
	Role     *Role     `json:"role,omitempty" validate:"omitempty,oneof=admin company_admin company_readonly_admin building_admin building_readonly_admin demo"`
	IsActive *bool     `json:"is_active,omitempty"`
}

// SMTPSettings is GET/PUT /smtp-settings; the password is never returned.
type SMTPSettings struct {
	Host        string    `json:"host" required:"true"`
	Port        int32     `json:"port" required:"true"`
	Secure      bool      `json:"secure" required:"true"`
	Username    string    `json:"username" required:"true"`
	FromAddress string    `json:"from_address" required:"true"`
	HasPassword bool      `json:"has_password" required:"true"`
	UpdatedAt   time.Time `json:"updated_at" required:"true"`
}

// SMTPPutRequest is PUT /smtp-settings; an empty password keeps the stored one.
type SMTPPutRequest struct {
	Host        string `json:"host" validate:"required,hostname|ip,max=253" required:"true"`
	Port        int32  `json:"port" validate:"required,min=1,max=65535" required:"true"`
	Secure      bool   `json:"secure"`
	Username    string `json:"username" validate:"max=254"`
	FromAddress string `json:"from_address" validate:"required,email,max=254" required:"true" format:"email"`
	Password    string `json:"password,omitempty" validate:"max=256"`
}

// SMTPTestRequest is POST /smtp-settings/test.
type SMTPTestRequest struct {
	To string `json:"to" validate:"required,email,max=254" required:"true" format:"email"`
}
