package v1

import (
	"net/http"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/tariff"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/publicforms"
)

// publicRoutes is 05 §17 (F12a R351–R356): unauthenticated, no tenant data.
func publicRoutes() []Route {
	return []Route{
		{Method: http.MethodPost, Pattern: "/public/bill-calculator", OperationID: "public.bill_calculator", Tag: "public", Access: Public,
			NoIdempotency: true, Summary: "Estimate a bill from the national tariff schedule, power charge included.",
			Request: dto.PublicBillRequest{}, Response: dto.PublicBill{}, Status: http.StatusOK, Handler: (*Handlers).publicBill},
		// R355: a per-IP limit on top of the global one.
		{Method: http.MethodPost, Pattern: "/public/contact", OperationID: "public.contact", Tag: "public", Access: Public, NoIdempotency: true,
			AuthLimit: AuthLimitByIP, Summary: "Contact form; e-mailed to the operator.", Request: dto.PublicContactRequest{},
			Status: http.StatusAccepted, Handler: (*Handlers).publicContact},
		{Method: http.MethodPost, Pattern: "/public/demo-request", OperationID: "public.demo_request", Tag: "public", Access: Public,
			NoIdempotency: true, AuthLimit: AuthLimitByIP, Summary: "Demo request form; e-mailed to the operator.", Request: dto.PublicDemoRequest{},
			Status: http.StatusAccepted, Handler: (*Handlers).publicDemo},
	}
}

func publicDec(d *dto.Decimal) *decimal.Decimal {
	if d == nil {
		return nil
	}
	v := d.Decimal
	return &v
}

func (h *Handlers) publicBill(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.PublicBillRequest) (any, error) {
		res, err := h.Tariffs.PublicBill(r.Context(), tariff.PublicInput{
			Group: model.DistributionUserGroup(req.UserGroup), Level: model.VoltageLevel(req.VoltageLevel), Term: model.TariffTerm(req.Term),
			MultiTime: req.MultiTime, Start: req.Start.Time, End: req.End.Time, Total: publicDec(req.TotalConsumption),
			T1: publicDec(req.T1), T2: publicDec(req.T2), T3: publicDec(req.T3), Demand: publicDec(req.Demand), ContractPower: publicDec(req.ContractPower),
		})
		if err != nil {
			return nil, err
		}
		return dto.PublicBill{Energy: dto.D(res.Energy), Distribution: dto.D(res.Distribution), Power: dto.D(res.Power), Overuse: dto.D(res.Overuse),
			VatBase: dto.D(res.VatBase), Vat: dto.D(res.Vat), Total: dto.D(res.Total), VatRate: dto.D(res.VatRate), Days: res.Days, Basis: res.Basis,
			Tariff: dto.PublicBillTariff{EffectiveFrom: dto.Date{Time: res.Row.EffectiveFrom}, GroupUsed: string(res.GroupUsed), Source: res.Row.Source}}, nil
	})
}

func (h *Handlers) publicContact(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusAccepted, func(req dto.PublicContactRequest) (any, error) {
		return nil, h.PublicForms.SubmitContact(r.Context(), publicforms.Contact{Name: req.Name, Email: req.Email, Phone: req.Phone,
			Subject: req.Subject, Message: req.Message, Website: req.Website, ElapsedMs: req.ElapsedMs})
	})
}

func (h *Handlers) publicDemo(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusAccepted, func(req dto.PublicDemoRequest) (any, error) {
		return nil, h.PublicForms.SubmitDemo(r.Context(), publicforms.Demo{Name: req.Name, Email: req.Email, Phone: req.Phone,
			Company: req.Company, Role: req.Role, Message: req.Message, Website: req.Website, ElapsedMs: req.ElapsedMs})
	})
}
