package player

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Marquee Player is installed from the hub itself: the device's browser
// opens /app, which explains the steps and serves the APK found next to the
// hub (AppFile), so a new player version ships by replacing that file.
// Installed players learn its checksum from the manifest and update
// themselves when theirs differs.

const AppName = "marquee-player.apk"

func (h *Handler) appPage(w http.ResponseWriter, r *http.Request) {
	lang := pageLang(w, r)
	data := struct {
		T       map[string]template.HTML
		Lang    string
		Hub     string
		Ready   bool
		Version string
		Size    string
		Date    string
		WebView bool
	}{T: appTexts[lang], Lang: lang, Hub: requestBase(r, h.cfg.PublicURL), WebView: isWebView(r.UserAgent())}
	if rel, ok := h.app.Release(); ok {
		data.Ready = true
		data.Version = rel.Version
		data.Size = fmt.Sprintf("%.1f MB", float64(rel.Size)/(1<<20))
		data.Date = rel.Mod.In(h.cfg.Loc).Format("02.01.2006")
	}
	var buf bytes.Buffer
	if err := appTmpl.Execute(&buf, data); err != nil {
		h.fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(buf.Bytes())
}

// pageLang is the language of the pages players and installers open:
// Bulgarian unless ?lang=en was chosen, which a cookie then remembers.
func pageLang(w http.ResponseWriter, r *http.Request) string {
	if l := r.URL.Query().Get("lang"); l == "bg" || l == "en" {
		http.SetCookie(w, &http.Cookie{Name: "lang", Value: l, Path: "/", MaxAge: 365 * 24 * 3600, SameSite: http.SameSiteLaxMode})
		return l
	}
	if c, err := r.Cookie("lang"); err == nil && c.Value == "en" {
		return "en"
	}
	return "bg"
}

// isWebView spots pages opened inside an app's WebView (such as the
// "WebView Browser Tester" on emulators) rather than a browser: those ignore
// downloads, so the button would silently do nothing.
func isWebView(ua string) bool {
	return strings.Contains(ua, "; wv)")
}

func (h *Handler) appFile(w http.ResponseWriter, r *http.Request) {
	if h.cfg.AppFile == "" {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(h.cfg.AppFile)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		h.fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.android.package-archive")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(AppName)+`"`)
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, AppName, st.ModTime(), f)
}

var appTmpl = template.Must(template.New("app").Parse(`<!doctype html>
<html lang="{{.Lang}}" translate="no"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="google" content="notranslate">
<title>Marquee Player</title>
<style>
html,body{margin:0;min-height:100%;background:#120c1f;color:#eee;font:18px/1.5 sans-serif}
body{background:radial-gradient(circle at 50% 20%,#2e1a52 0,#120c1f 70%);padding:24px 16px}
main{max-width:720px;margin:0 auto}
header{display:flex;align-items:center;justify-content:space-between;gap:12px}
.logo{display:flex;align-items:center;gap:12px;font-size:28px;font-weight:700}
.logo svg{width:44px;height:44px}
.langs{display:flex;gap:4px}
.langs a{color:#c4b5fd;text-decoration:none;padding:6px 12px;border-radius:8px;font-size:16px;font-weight:700}
.langs a.on{background:#2b2142;color:#fff}
h1{font-size:26px;margin:28px 0 8px}
.dl{display:inline-block;margin:16px 0 4px;padding:16px 32px;border-radius:12px;background:#7c3aed;color:#fff;
text-decoration:none;font-size:22px;font-weight:700}
.ver{font-size:17px}.ver b{color:#c4b5fd}
.muted{opacity:.65;font-size:15px}
ol{padding-left:24px}li{margin:10px 0}
code{background:#2b2142;color:#c4b5fd;padding:2px 8px;border-radius:6px;font-size:20px;word-break:break-all}
.warn{background:#3b2a12;border-radius:10px;padding:12px 16px}
</style></head><body><main>
<header>
<div class="logo"><svg viewBox="0 0 64 64"><rect width="64" height="64" rx="16" fill="#7c3aed"/><circle cx="32" cy="32" r="22" fill="none" stroke="#fff" stroke-width="3" opacity=".28"/><circle cx="32" cy="32" r="14" fill="none" stroke="#fff" stroke-width="3.5" opacity=".6"/><circle cx="32" cy="32" r="7" fill="#fff"/></svg>marquee</div>
<nav class="langs"><a href="?lang=bg"{{if eq .Lang "bg"}} class="on"{{end}}>БГ</a><a href="?lang=en"{{if eq .Lang "en"}} class="on"{{end}}>EN</a></nav>
</header>
<h1>{{.T.title}}</h1>
{{if .WebView}}
<p class="warn">{{.T.webview}} <code>{{.Hub}}/app</code> {{.T.webview2}}</p>
{{end}}
{{if .Ready}}
<a class="dl" href="/app/marquee-player.apk">{{.T.download}}</a>
{{if .Version}}<div class="ver">{{.T.version}} <b>{{.Version}}</b></div>{{end}}
<div class="muted">{{.Size}} · {{.T.from}} {{.Date}} · {{.T.android}}</div>
{{else}}
<p class="warn">{{.T.missing}}</p>
{{end}}
<h1>{{.T.install}}</h1>
<ol>
<li>{{.T.s1}}</li>
<li>{{.T.s2}}</li>
<li>{{.T.s3}}</li>
<li>{{.T.s4}}</li>
<li>{{.T.s5}}</li>
<li>{{.T.s6}} <code>{{.Hub}}</code></li>
<li>{{.T.s7}}</li>
<li>{{.T.s8}}</li>
</ol>
<p class="muted">{{.T.notes}}</p>
</main></body></html>`))

// appTexts are the /app page in each language; values may hold markup.
var appTexts = map[string]map[string]template.HTML{
	"bg": {
		"title":    "Marquee Player за Android",
		"webview":  "Тази страница е отворена в приложение, което не може да сваля файлове (WebView). Отворете адреса",
		"webview2": "в <b>Chrome</b> или в браузъра на устройството.",
		"download": "Свали плейъра",
		"version":  "Актуална версия, която плейърите ще получат:",
		"from":     "от",
		"android":  "Android 7.1 и по-нов",
		"missing":  "Плейърът още не е сложен на хъба: копирайте файла <b>marquee-player.apk</b> в папката на marquee.exe.",
		"install":  "Инсталиране",
		"s1":       "Натиснете <b>„Свали плейъра“</b>. Първия път Chrome иска достъп до файловете: <b>„Продължи“ → „Разреши“</b>.",
		"s2":       "Chrome пита най-долу на екрана дали да запази файла („може да навреди на устройството“): натиснете <b>OK</b>. Без този отговор файлът не се сваля.",
		"s3":       "Отворете файла: <b>„Отвори“</b> в съобщението долу или от известието горе („Изтеглянето завърши“).",
		"s4":       "Ако Android откаже: <b>Настройки → Сигурност → Непознати източници</b>, включете и отворете файла отново.",
		"s5":       "Натиснете <b>„Инсталиране“</b>, после <b>„Отваряне“</b>.",
		"s6":       "Въведете адреса на хъба:",
		"s7":       "На екрана излиза код. Въведете го в админ панела: <b>Чакащи устройства</b>.",
		"s8":       "Натиснете бутона <b>„Начало“</b> (Home) и изберете <b>Marquee Player → „Винаги“</b>. Така плейърът тръгва сам при всяко включване.",
		"notes":    "Настройките на плейъра (адресът на хъба) се отварят, като задържите клик 3 секунди където и да е на екрана. Инсталираните плейъри се обновяват сами до актуалната версия; ръчно: свалете и инсталирайте отгоре, без да деинсталирате — всичко се запазва.",
	},
	"en": {
		"title":    "Marquee Player for Android",
		"webview":  "This page is open in an app that cannot download files (WebView). Open",
		"webview2": "in <b>Chrome</b> or the device's browser.",
		"download": "Download the player",
		"version":  "Current version players will receive:",
		"from":     "from",
		"android":  "Android 7.1 or newer",
		"missing":  "The player is not on the hub yet: copy <b>marquee-player.apk</b> into the folder of marquee.exe.",
		"install":  "Installation",
		"s1":       "Tap <b>“Download the player”</b>. The first time, Chrome asks for access to files: <b>“Continue” → “Allow”</b>.",
		"s2":       "At the bottom of the screen Chrome asks whether to keep the file (“may harm your device”): tap <b>OK</b>. Without this answer the file is not downloaded.",
		"s3":       "Open the file: <b>“Open”</b> in the message at the bottom or from the notification at the top (“Download complete”).",
		"s4":       "If Android refuses: <b>Settings → Security → Unknown sources</b>, turn it on and open the file again.",
		"s5":       "Tap <b>“Install”</b>, then <b>“Open”</b>.",
		"s6":       "Enter the hub address:",
		"s7":       "A code appears on the screen. Enter it in the admin panel: <b>Pending devices</b>.",
		"s8":       "Press the <b>Home</b> button and choose <b>Marquee Player → “Always”</b>, so the player starts by itself whenever the device is switched on.",
		"notes":    "The player settings (the hub address) open when you hold a click for 3 seconds anywhere on the screen. Installed players update themselves to the current version; by hand: download and install over the old one without uninstalling — everything is kept.",
	},
}
