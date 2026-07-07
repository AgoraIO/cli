package cli

import (
	"html"
	"strings"
)

// callbackPageConfig carries the login region used to brand the browser callback page.
type callbackPageConfig struct {
	Region string
}

// loginPageContent contains the region-specific copy rendered on the OAuth callback page.
type loginPageContent struct {
	PageClass     string
	Title         string
	Message       string
	ActionLabel   string
	PrimaryAction string
	Safety        string
}

// renderOAuthCallbackSuccessPage renders the browser page shown after a successful OAuth callback.
func renderOAuthCallbackSuccessPage(config callbackPageConfig) string {
	content := loginPageContentForRegion(config.Region)
	return renderOAuthCallbackPage(content, false, "", "")
}

// renderOAuthCallbackErrorPage renders a branded browser page for OAuth callback errors.
func renderOAuthCallbackErrorPage(config callbackPageConfig, title, message string) string {
	content := loginPageContentForRegion(config.Region)
	return renderOAuthCallbackPage(content, true, title, message)
}

// loginPageContentForRegion returns the final login-page copy for the active control-plane region.
func loginPageContentForRegion(region string) loginPageContent {
	if normalizeContextRegion(region) == regionCN {
		return loginPageContent{
			PageClass:     "shengwang-page",
			Title:         "你已成功登录声网 CLI",
			Message:       "此浏览器步骤已完成。CLI 现在可以将此账号作为当前本地配置继续使用。",
			ActionLabel:   "回到终端后确认当前登录状态。",
			PrimaryAction: "agora auth status",
			Safety:        "你可以关闭此页面，回到终端继续操作。",
		}
	}

	return loginPageContent{
		PageClass:     "agora-page",
		Title:         "You are now authenticated with Agora CLI",
		Message:       "This browser step completed successfully. The CLI can now use this account as your active local configuration.",
		ActionLabel:   "Return to your terminal and confirm the active account.",
		PrimaryAction: "agora auth status",
		Safety:        "You can close this window and return to your terminal.",
	}
}

// renderOAuthCallbackPage builds the complete OAuth callback HTML document.
func renderOAuthCallbackPage(content loginPageContent, isError bool, errorTitle, errorMessage string) string {
	title := content.Title
	message := content.Message
	if isError {
		title = valueOrDefault(errorTitle, "Login could not be completed")
		message = valueOrDefault(errorMessage, "Return to your terminal for details.")
	}

	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Agora CLI Login</title><style>`)
	b.WriteString(loginPageCSS(content.PageClass))
	b.WriteString(`</style></head><body><main class="page `)
	b.WriteString(escapeAttr(content.PageClass))
	if isError {
		b.WriteString(` is-error`)
	}
	b.WriteString(`"><section class="card"><div class="brand">`)
	b.WriteString(brandLogoHTML(content.PageClass))
	b.WriteString(`<span>CLI</span></div><div class="hero"><h1>`)
	b.WriteString(escapeText(title))
	b.WriteString(`</h1><p class="message">`)
	b.WriteString(escapeText(message))
	b.WriteString(`</p></div>`)
	if !isError {
		b.WriteString(`<div class="next"><p>`)
		b.WriteString(escapeText(content.ActionLabel))
		b.WriteString(`</p><code>`)
		b.WriteString(escapeText(content.PrimaryAction))
		b.WriteString(`</code></div>`)
	}
	b.WriteString(`<p class="safety">`)
	b.WriteString(escapeText(content.Safety))
	b.WriteString(`</p></section></main></body></html>`)

	return b.String()
}

// loginPageCSS returns shared page CSS plus the theme for the active login region.
func loginPageCSS(pageClass string) string {
	var b strings.Builder
	b.WriteString(`
*{box-sizing:border-box}
body{margin:0;min-height:100vh;font-family:ui-sans-serif,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;-webkit-font-smoothing:antialiased}
.page{min-height:100vh;display:grid;place-items:center;padding:64px 24px;color:var(--ink);background:var(--bg)}
.card{width:min(760px,100%);padding:40px;border:1px solid var(--line);border-radius:20px;background:var(--panel);box-shadow:var(--shadow)}
.brand{display:inline-flex;align-items:center;gap:10px;color:var(--ink);font-size:16px;font-weight:700;letter-spacing:-.01em}
.brand-logo{display:inline-flex;align-items:center;flex:0 0 auto}
.brand-logo svg{display:block;width:100%;height:100%}
.brand-logo img{display:block;width:100%;height:auto}
.brand-logo-agora{width:82px;max-height:32px}
.hero{margin-top:36px}
h1{margin:0;max-width:700px;font-size:clamp(26px,3vw,31px);line-height:1.22;letter-spacing:-.03em}
.message{max-width:680px;margin:18px 0 0;color:var(--muted);font-size:16px;line-height:1.65}
.next{display:grid;grid-template-columns:1fr auto;gap:18px;align-items:center;margin-top:28px;padding:14px 16px;border:1px solid var(--next-line);border-radius:12px;background:var(--next-bg)}
.next p{margin:0;color:var(--next-text);font-size:14px;font-weight:600}
code{padding:5px 9px;border:1px solid var(--code-line);border-radius:8px;background:var(--code-bg);color:var(--primary);font:700 14px ui-monospace,SFMono-Regular,Menlo,monospace;white-space:nowrap}
.safety{margin:20px 0 0;color:var(--muted);font-size:13px;line-height:1.6}
.is-error .next{display:none}
@media (max-width:900px){
  .page{padding:32px 18px}
  .card{padding:28px}
  .next{grid-template-columns:1fr}
  code{white-space:normal;word-break:break-word}
}
`)

	if pageClass == "shengwang-page" {
		b.WriteString(`
.shengwang-page{--ink:#152033;--muted:#617083;--primary:#1469ff;--brand:#1469ff;--line:#dce7f5;--panel:#fff;--shadow:0 20px 55px rgba(21,45,85,.1);--next-line:#dce7f5;--next-bg:#f7fbff;--next-text:#46576c;--code-line:#cfe0f8;--code-bg:#eef6ff;--bg:linear-gradient(180deg,#f6faff 0%,#fff 58%,#f9fbfd 100%)}
.brand-logo-cn{width:88px;height:auto;color:var(--brand)}
.brand-logo-cn svg{width:88px;height:auto}
`)
		return b.String()
	}

	b.WriteString(`
.agora-page{--ink:#172033;--muted:#647084;--primary:#09976f;--line:#dfe5ee;--panel:#fff;--shadow:0 20px 55px rgba(22,33,55,.1);--next-line:#dfe5ee;--next-bg:#f8fafc;--next-text:#4c596c;--code-line:#d8e2ee;--code-bg:#f3f6fa;--bg:linear-gradient(180deg,#f8fafc 0%,#fff 58%,#f6f8fb 100%)}
`)
	return b.String()
}

// brandLogoHTML returns the official region-specific brand mark used in the callback page.
func brandLogoHTML(pageClass string) string {
	if pageClass == "shengwang-page" {
		return `<span class="brand-logo brand-logo-cn" aria-hidden="true"><svg viewBox="0 0 52 27" fill="currentColor" xmlns="http://www.w3.org/2000/svg"><path d="M24.326 3.90545V1.16386H13.9235V-0.480469L13.6627 -0.44245C12.7048 -0.304103 11.7427 0.232389 11.2115 1.16491H0.375V3.90545H10.7922V5.73248H1.81972V8.47302H22.896V5.73248H13.9235V3.90545H24.326Z"/><path d="M51.7478 3.9912C51.5683 2.49895 50.3633 1.31719 48.8552 1.16406H27.7969V25.4223L28.0577 25.3843C29.4866 25.1773 30.9282 24.0864 30.9282 22.1136V3.9046H47.8921C48.2923 3.9046 48.6165 4.22671 48.6165 4.62485C48.6165 4.62485 48.6176 20.9878 48.6176 20.9899C48.6176 22.3586 47.8921 23.5615 46.8032 24.2374L48.9312 26.5576C50.6389 25.2914 51.7468 23.2669 51.7468 20.9899L51.7478 3.9912Z"/><path d="M47.8279 6.98899L47.6082 6.91507C46.4053 6.50953 44.836 6.79256 44.045 8.33867L43.1114 10.1615L42.1493 8.2827C42.1482 8.28164 42.1482 8.28164 42.1482 8.28059C41.8832 7.7631 41.5304 7.38714 41.1344 7.13156C41.1302 7.1284 41.1249 7.12523 41.1196 7.12206C40.7257 6.88444 40.2631 6.74609 39.7699 6.74609C39.2767 6.74609 38.8142 6.88339 38.4202 7.12206C38.4044 7.13157 38.3896 7.14107 38.3738 7.15163C37.9925 7.40615 37.6535 7.77155 37.3958 8.27214C37.3948 8.27319 37.3948 8.27425 37.3948 8.27531C37.3937 8.27742 37.3937 8.27847 37.3916 8.28059L36.4285 10.1604L35.4949 8.33761C34.7028 6.79045 33.1345 6.50742 31.9306 6.91401L31.7109 6.98794L34.8824 13.1798L31.7109 19.3716L31.9306 19.4455C33.1345 19.8511 34.7028 19.568 35.4949 18.0219L36.4285 16.1991L37.3916 18.0789C37.3927 18.0811 37.3937 18.0821 37.3948 18.0842C37.3958 18.0853 37.3958 18.0853 37.3958 18.0874C37.6535 18.588 37.9925 18.9534 38.3738 19.2079C38.3896 19.2174 38.4044 19.228 38.4202 19.2375C38.8142 19.4751 39.2767 19.6134 39.7699 19.6134C40.2631 19.6134 40.7257 19.4761 41.1196 19.2375C41.1249 19.2343 41.1302 19.2311 41.1344 19.228C41.5304 18.9724 41.8832 18.5964 42.1482 18.0789C42.1493 18.0779 42.1493 18.0779 42.1493 18.0768L43.1114 16.1981L44.045 18.0209C44.837 19.568 46.4053 19.8511 47.6082 19.4445L47.8279 19.3705L44.6564 13.1787L47.8279 6.98899ZM39.7699 16.6839L37.9756 13.1808L39.7699 9.67779L41.5642 13.1808L39.7699 16.6839Z"/><path d="M22.8896 10.3047H1.8133C1.8133 10.3047 1.81435 20.9807 1.81435 20.9944C1.81435 22.3631 1.08882 23.5659 0 24.2418L2.12801 26.5621C3.8357 25.2958 4.94353 23.2724 4.94353 20.9944V18.3954H19.9896C21.4924 18.2433 22.6974 17.0657 22.8822 15.5798L22.8896 10.3047ZM4.94459 13.0452H10.7858V15.6538H4.94459V13.0452ZM19.7509 14.9684C19.7509 15.3475 19.4425 15.6538 19.0613 15.6538H13.9171V13.0452H19.752L19.7509 14.9684Z"/></svg></span>`
	}

	return `<span class="brand-logo brand-logo-agora" aria-hidden="true"><img src="https://cdn.prod.website-files.com/660affa848e8af81bdd03909/66ab7f671fb90c022fb7f1dc_Agora%20Logo%20Crisp.webp" alt=""></span>`
}

// escapeText escapes user-visible text before it is inserted into HTML.
func escapeText(value string) string {
	return html.EscapeString(value)
}

// escapeAttr escapes attribute values before they are inserted into HTML.
func escapeAttr(value string) string {
	return html.EscapeString(value)
}
