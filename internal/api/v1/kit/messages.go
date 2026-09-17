package kit

import (
	"net/http"
	"strconv"
	"strings"
)

// messages is the localised text of every error code the API emits (R152).
var messages = map[string][2]string{ // code → {tr, en}
	"not_found":                  {"Kayıt bulunamadı.", "Not found."},
	"unauthorized":               {"Oturum açmanız gerekiyor.", "Authentication required."},
	"forbidden":                  {"Bu işlem için yetkiniz yok.", "You are not allowed to do this."},
	"validation_failed":          {"Girilen bilgiler geçersiz.", "The submitted data is invalid."},
	"conflict":                   {"İşlem mevcut durumla çakışıyor.", "The request conflicts with the current state."},
	"unavailable":                {"Hizmet şu anda kullanılamıyor.", "The service is currently unavailable."},
	"internal":                   {"Beklenmeyen bir hata oluştu.", "An unexpected error occurred."},
	"rate_limited":               {"Çok fazla deneme yapıldı. Lütfen daha sonra tekrar deneyin.", "Too many attempts. Please try again later."},
	"method_not_allowed":         {"Bu yöntem desteklenmiyor.", "Method not allowed."},
	"unknown_parameter":          {"Bilinmeyen sorgu parametresi.", "Unknown query parameter."},
	"invalid_parameters":         {"Sorgu parametreleri geçersiz.", "Invalid query parameters."},
	"invalid_body":               {"İstek gövdesi geçersiz.", "The request body is invalid."},
	"body_too_large":             {"İstek gövdesi çok büyük.", "The request body is too large."},
	"unsupported_media_type":     {"İstek içerik türü desteklenmiyor.", "Unsupported content type."},
	"invalid_limit":              {"Sayfa boyutu geçersiz.", "Invalid page size."},
	"invalid_cursor":             {"Sayfa imleci geçersiz.", "Invalid page cursor."},
	"idempotency_key_mismatch":   {"Bu işlem anahtarı farklı bir istekle kullanılmış.", "This idempotency key was used with a different request."},
	"idempotency_in_progress":    {"Aynı işlem hâlâ sürüyor.", "The same request is still in progress."},
	"invalid_credentials":        {"E-posta veya şifre hatalı.", "Incorrect e-mail or password."},
	"token_expired":              {"Oturum süresi doldu.", "The session token has expired."},
	"token_invalid":              {"Oturum bilgisi geçersiz.", "The session token is invalid."},
	"token_rotated":              {"Oturum yenilendi, lütfen tekrar deneyin.", "The session was refreshed, please retry."},
	"session_revoked":            {"Oturumunuz sonlandırıldı. Lütfen tekrar giriş yapın.", "Your session has ended. Please sign in again."},
	"device_mismatch":            {"Oturumunuz başka bir cihazda kullanıldığı için sonlandırıldı. Lütfen tekrar giriş yapın.", "Your session was ended because it was used from another device. Please sign in again."},
	"email_taken":                {"Bu e-posta adresi başka bir kullanıcıya ait.", "This e-mail address belongs to another user."},
	"company_name_taken":         {"Bu adla bir şirket zaten var.", "A company with this name already exists."},
	"cannot_delete_own_company":  {"Kendi şirketinizi silemezsiniz.", "You cannot delete your own company."},
	"cannot_modify_self":         {"Kendi rolünüzü, durumunuzu değiştiremez veya hesabınızı silemezsiniz.", "You cannot change your own role or status, or delete yourself."},
	"smtp_test_failed":           {"Test e-postası gönderilemedi.", "The test e-mail could not be sent."},
	"building_has_analyzers":     {"Analizör bağlı bir bina silinemez.", "A building with analyzers attached cannot be deleted."},
	"integration_not_configured": {"Bu analizör için entegrasyon tanımlanmamış.", "No integration is configured for this analyzer."},
	"refresh_in_progress":        {"Bu analizör için yenileme zaten sırada.", "A refresh for this analyzer is already queued."},
	"building_sector_missing":    {"Karşılaştırma için binanın sektörü tanımlı olmalıdır.", "The building needs a sector to be compared."},
	"reset_token_invalid":        {"Şifre sıfırlama bağlantısı geçersiz veya süresi dolmuş.", "The password reset link is invalid or has expired."},
}

// Message returns the localised text for code, falling back to "internal".
func Message(code, locale string) string {
	m, ok := messages[code]
	if !ok {
		m = messages["internal"]
	}
	if locale == "en" {
		return m[1]
	}
	return m[0]
}

// HasMessage reports whether code has catalogue entries.
func HasMessage(code string) bool { _, ok := messages[code]; return ok }

// Codes returns every catalogued code.
func Codes() []string {
	out := make([]string, 0, len(messages))
	for c := range messages {
		out = append(out, c)
	}
	return out
}

// RegisterMessages adds codes owned by later endpoint groups; duplicates panic.
func RegisterMessages(m map[string][2]string) {
	for code, text := range m {
		if _, dup := messages[code]; dup {
			panic("kit: duplicate message code " + code)
		}
		messages[code] = text
	}
}

// Locale picks tr or en from Accept-Language q-values; tr is the default.
func Locale(r *http.Request) string {
	best, bestQ := "tr", 0.0
	for _, part := range strings.Split(r.Header.Get("Accept-Language"), ",") {
		tag, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		q := 1.0
		if v, ok := strings.CutPrefix(strings.TrimSpace(params), "q="); ok {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				q = f
			}
		}
		lang := strings.ToLower(strings.SplitN(tag, "-", 2)[0])
		if (lang == "tr" || lang == "en") && q > bestQ {
			best, bestQ = lang, q
		}
	}
	return best
}
