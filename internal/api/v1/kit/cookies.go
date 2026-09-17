package kit

import (
	"net/http"
	"time"
)

// Auth cookie names (R143).
const (
	AccessCookie  = "ekokod_at"
	RefreshCookie = "ekokod_rt"
)

// SetAuthCookies writes both session cookies. The refresh cookie is a browser
// session cookie unless remember is set (R143).
func SetAuthCookies(w http.ResponseWriter, access string, accessExpires time.Time, refresh string, sessionExpires time.Time, remember bool, now time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name: AccessCookie, Value: access, Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode,
		MaxAge: max(1, int(accessExpires.Sub(now).Seconds())),
	})
	rt := &http.Cookie{Name: RefreshCookie, Value: refresh, Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode}
	if remember {
		rt.MaxAge = max(1, int(sessionExpires.Sub(now).Seconds()))
	}
	http.SetCookie(w, rt)
}

// ClearAuthCookies expires both session cookies.
func ClearAuthCookies(w http.ResponseWriter) {
	for _, name := range []string{AccessCookie, RefreshCookie} {
		http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", HttpOnly: true, Secure: true,
			SameSite: http.SameSiteStrictMode, MaxAge: -1})
	}
}
