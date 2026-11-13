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

const AppName = "marquee-player.apk"

func (h *Handler) appPage(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Hub     string
		Ready   bool
		Size    string
		Date    string
		WebView bool
	}{Hub: requestBase(r, h.cfg.PublicURL), WebView: isWebView(r.UserAgent())}
	if st, err := os.Stat(h.cfg.AppFile); err == nil && h.cfg.AppFile != "" {
		data.Ready = true
		data.Size = fmt.Sprintf("%.1f MB", float64(st.Size())/(1<<20))
		data.Date = st.ModTime().In(h.cfg.Loc).Format("02.01.2006")
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
<html lang="bg" translate="no"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="google" content="notranslate">
<title>Marquee Player</title>
<style>
html,body{margin:0;min-height:100%;background:#120c1f;color:#eee;font:18px/1.5 sans-serif}
body{background:radial-gradient(circle at 50% 20%,#2e1a52 0,#120c1f 70%);padding:24px 16px}
main{max-width:720px;margin:0 auto}
.logo{display:flex;align-items:center;gap:12px;font-size:28px;font-weight:700}
.logo svg{width:44px;height:44px}
h1{font-size:26px;margin:28px 0 8px}
.dl{display:inline-block;margin:16px 0 4px;padding:16px 32px;border-radius:12px;background:#7c3aed;color:#fff;
text-decoration:none;font-size:22px;font-weight:700}
.muted{opacity:.65;font-size:15px}
ol{padding-left:24px}li{margin:10px 0}
code{background:#2b2142;color:#c4b5fd;padding:2px 8px;border-radius:6px;font-size:20px;word-break:break-all}
.warn{background:#3b2a12;border-radius:10px;padding:12px 16px}
</style></head><body><main>
<div class="logo"><svg viewBox="0 0 64 64"><rect width="64" height="64" rx="16" fill="#7c3aed"/><circle cx="32" cy="32" r="22" fill="none" stroke="#fff" stroke-width="3" opacity=".28"/><circle cx="32" cy="32" r="14" fill="none" stroke="#fff" stroke-width="3.5" opacity=".6"/><circle cx="32" cy="32" r="7" fill="#fff"/></svg>marquee</div>
<h1>Marquee Player за Android</h1>
{{if .WebView}}
<p class="warn">Тази страница е отворена в приложение, което не може да сваля файлове (WebView).
Отворете адреса <code>{{.Hub}}/app</code> в <b>Chrome</b> или в браузъра на устройството.</p>
{{end}}
{{if .Ready}}
<a class="dl" href="/app/marquee-player.apk">Свали плейъра</a>
<div class="muted">{{.Size}} · от {{.Date}} · Android 7.1 и по-нов</div>
{{else}}
<p class="warn">Плейърът още не е сложен на хъба: копирайте файла <b>marquee-player.apk</b> в папката на marquee.exe.</p>
{{end}}
<h1>Инсталиране</h1>
<ol>
<li>Натиснете <b>„Свали плейъра“</b>. Първия път Chrome иска достъп до файловете: <b>„Продължи“ → „Разреши“</b>.</li>
<li>Chrome пита най-долу на екрана дали да запази файла („може да навреди на устройството“): натиснете <b>OK</b>.
Без този отговор файлът не се сваля.</li>
<li>Отворете файла: <b>„Отвори“</b> в съобщението долу или от известието горе („Изтеглянето завърши“).</li>
<li>Ако Android откаже: <b>Настройки → Сигурност → Непознати източници</b>, включете и отворете файла отново.</li>
<li>Натиснете <b>„Инсталиране“</b>, после <b>„Отваряне“</b>.</li>
<li>Въведете адреса на хъба: <code>{{.Hub}}</code></li>
<li>На екрана излиза код. Въведете го в админ панела: <b>Чакащи устройства</b>.</li>
<li>Натиснете бутона <b>„Начало“</b> (Home) и изберете <b>Marquee Player → „Винаги“</b>. Така плейърът тръгва сам при всяко включване.</li>
</ol>
<p class="muted">Настройките на плейъра (адресът на хъба) се отварят, като задържите клик 3 секунди където и да е на екрана.
При нова версия: свалете и инсталирайте отгоре, без да деинсталирате — всичко се запазва.</p>
</main></body></html>`))
