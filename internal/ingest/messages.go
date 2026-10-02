package ingest

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
)

// The Messages screen shows these texts verbatim, so they are sentences an
// operator can act on, in the product's language, naming the meter by its
// installation number (R510). Machine-readable detail stays in the metadata.

var rejectText = map[RejectReason]string{
	RejectUnparseableTs:        "zamanı okunamayan",
	RejectFuture:               "gelecek tarihli",
	RejectAllNull:              "boş",
	RejectNegative:             "negatif değerli",
	RejectSanityJump:           "olağan dışı sıçrayan",
	RejectConflictingDuplicate: "çelişen yinelenmiş",
	RejectWrongAnalyzer:        "başka sayaca ait",
	RejectWrongKind:            "başka türden",
}

var warningText = map[string]string{
	integration.WarnMultiplierFallback:    "sayaç çarpanı sağlayıcıdan alınamadı, kayıtlı çarpan kullanıldı",
	integration.WarnUnparseableRow:        "sağlayıcının gönderdiği satır okunamadı ve atlandı",
	integration.WarnGenerationIntervalNil: "okumada üretim değeri yok",
	integration.WarnMissingPriceHours:     "saat için fiyat verisi eksik",
}

func inactiveMessage(a model.Analyzer) string {
	return fmt.Sprintf("%s: analizör pasif olduğu için veri çekilmedi.", a.InstallationNumber)
}

func multiplierMessage(a model.Analyzer) string {
	return fmt.Sprintf("%s: sayaç çarpanı %s olarak güncellendi. Bu değişiklik faturaları etkiler.", a.InstallationNumber, a.MeterMultiplier.String())
}

// rejectedMessage lists the counts by reason, largest first.
func rejectedMessage(a model.Analyzer, total int32, byReason map[RejectReason]int32) string {
	reasons := make([]RejectReason, 0, len(byReason))
	for r := range byReason {
		reasons = append(reasons, r)
	}
	sort.Slice(reasons, func(i, j int) bool {
		if byReason[reasons[i]] != byReason[reasons[j]] {
			return byReason[reasons[i]] > byReason[reasons[j]]
		}
		return reasons[i] < reasons[j]
	})
	parts := make([]string, 0, len(reasons))
	for _, r := range reasons {
		text, ok := rejectText[r]
		if !ok {
			text = string(r)
		}
		parts = append(parts, fmt.Sprintf("%d %s", byReason[r], text))
	}
	return fmt.Sprintf("%s: %d okuma kaydedilmedi (%s).", a.InstallationNumber, total, strings.Join(parts, ", "))
}

func warningMessage(a model.Analyzer, code string, count int32) string {
	text, ok := warningText[code]
	if !ok {
		text = code
	}
	return fmt.Sprintf("%s: %d %s.", a.InstallationNumber, count, text)
}

// failureMessage says what went wrong in plain words and keeps the technical
// text, already redacted, for whoever has to look further.
func failureMessage(installation string, cause error, errText string) string {
	return fmt.Sprintf("%s: veri çekilemedi — %s (%s)", installation, failureReason(cause), errText)
}

func failureReason(cause error) string {
	switch {
	case errors.Is(cause, integration.ErrAuth):
		return "sağlayıcı kimlik bilgilerini kabul etmedi"
	case errors.Is(cause, integration.ErrRateLimited):
		return "sağlayıcı istek sınırına takıldı"
	case errors.Is(cause, integration.ErrUpstreamUnavailable):
		return "sağlayıcıya ulaşılamadı"
	case errors.Is(cause, integration.ErrMalformedPayload):
		return "sağlayıcının yanıtı anlaşılamadı"
	case errors.Is(cause, integration.ErrNotFound):
		return "sağlayıcıda kayıt bulunamadı"
	case errors.Is(cause, integration.ErrConfig):
		return "entegrasyon ayarı eksik ya da hatalı"
	default:
		return "beklenmeyen bir hata oluştu"
	}
}
