package report

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/mail"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/secret"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

const deliveryCategory = "report-delivery"

// Deliverer implements job.ReportDeliverer (R266).
type Deliverer struct {
	Reports store.ReportRepository
	Files   Files
	SMTP    store.SMTPRepository
	Mail    mail.Sender
	Ops     store.OpsRepository
	Clock   clock.Clock
	// Locale is the text alternative's language; the stored subject and
	// body are the company's outbound locale already.
	Locale string
}

// Deliver sends one report. A delivery failure is recorded, not retried
// (R222's shape): the job answers with a closed code and Messages says why.
func (d Deliverer) Deliver(ctx context.Context, p job.ReportDeliverPayload) error {
	sc := store.SystemScope(p.CompanyID)
	taskID := job.ReportDeliverTaskID(p)
	scopeJSON, _ := json.Marshal(map[string]any{"report_id": p.ReportID, "recipients": len(p.To)})
	run, err := d.Ops.StartRun(ctx, sc, model.JobRun{CompanyID: &p.CompanyID, JobType: job.TypeReportDeliver, Scope: scopeJSON,
		StartedAt: d.Clock.Now().UTC(), Status: "running", TaskID: &taskID})
	if err != nil {
		return err
	}
	fail := func(code, reason string, cause error, hide ...string) error {
		text := reason
		if cause != nil {
			text += ": " + secret.Redact(cause.Error(), hide)
		}
		d.message(ctx, sc, p, "error", "Rapor e-postası gönderilemedi: "+text)
		detail, _ := json.Marshal(map[string]string{"code": code})
		_, err := d.Ops.FinishRun(ctx, sc, run.ID, "failed", 0, 0, 1, &text, detail, d.Clock.Now().UTC())
		return err
	}

	rp, err := d.Reports.Get(ctx, sc, p.ReportID)
	if err != nil || rp.Status != model.ReportStatusCompleted || rp.EmailSubject == nil || rp.EmailBody == nil {
		return fail("report_not_ready", "rapor hazır değil", nil)
	}
	settings, err := d.SMTP.Get(ctx, sc)
	if err != nil {
		return fail("smtp_not_configured", "şirketin SMTP ayarları yok", nil)
	}
	password, err := d.SMTP.OpenPassword(ctx, sc)
	if err != nil {
		return fail("smtp_not_configured", "SMTP parolası okunamadı", nil)
	}
	pl, err := payload(rp)
	if err != nil {
		return fail("report_not_ready", "rapor hazır değil", nil)
	}
	msg := mail.Message{To: p.To, Subject: *rp.EmailSubject, HTML: *rp.EmailBody, Text: EmailText(pl, buildingNames(pl), d.locale())}
	for _, a := range []struct{ format, contentType string }{
		{FormatPDF, "application/pdf"},
		{FormatExcel, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
	} {
		data, err := d.Files.Read(ctx, rp, a.format, "tr")
		if err != nil {
			return fail("report_not_ready", "rapor dosyası üretilemedi", err)
		}
		msg.Attachments = append(msg.Attachments, mail.Attachment{Filename: FileName(rp, a.format), ContentType: a.contentType, Data: data})
	}
	if err := d.Mail.Send(ctx, settings, password, msg); err != nil {
		// An SMTP reply can quote the credentials it rejected.
		return fail("delivery_failed", "e-posta gönderilemedi", err, secret.Fragments(string(password))...)
	}
	d.message(ctx, sc, p, "success", fmt.Sprintf("Rapor e-postası gönderildi: %s, %d alıcı.", rp.Period, len(p.To)))
	_, err = d.Ops.FinishRun(ctx, sc, run.ID, "success", 1, 0, 0, nil, []byte(`{}`), d.Clock.Now().UTC())
	return err
}

func (d Deliverer) locale() string {
	if d.Locale == "en" {
		return "en"
	}
	return "tr"
}

// message writes one Messages row; failing to write it must not resend.
func (d Deliverer) message(ctx context.Context, sc store.Scope, p job.ReportDeliverPayload, status, text string) {
	company, related, id := p.CompanyID, "report", p.ReportID
	_, _ = d.Ops.AppendMessage(ctx, sc, model.OperationalMessage{CompanyID: &company, Kind: "job", Category: deliveryCategory,
		Status: status, Message: text, RelatedType: &related, RelatedID: &id, CreatedAt: d.Clock.Now()})
}
