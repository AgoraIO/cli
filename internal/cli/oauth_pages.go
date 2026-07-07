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
	b.WriteString(loginPageCSS())
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

// loginPageCSS returns the shared CSS for the final global and China login pages.
func loginPageCSS() string {
	return `
*{box-sizing:border-box}
body{margin:0;min-height:100vh;font-family:ui-sans-serif,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}
.page{min-height:100vh;display:grid;place-items:center;padding:90px 28px 48px;position:relative;overflow:hidden;color:var(--ink);background:var(--bg)}
.page:before{content:"";position:absolute;inset:0;pointer-events:none}
.card{width:min(900px,100%);position:relative;z-index:2;min-height:auto;padding:42px;border:1px solid var(--line);border-radius:30px;background:var(--panel);box-shadow:var(--shadow);backdrop-filter:blur(18px)}
.brand{display:inline-flex;align-items:center;gap:10px;color:var(--ink);font-size:20px;font-weight:950;letter-spacing:-.02em}
.brand-logo{display:inline-flex;align-items:center;flex:0 0 auto}
.brand-logo svg{display:block;width:100%;height:100%}
.brand-logo img{display:block;width:100%;height:auto}
.brand-logo-agora{width:86px;max-height:34px}
.brand-logo-cn{justify-content:center;width:58px;height:58px;border-radius:16px;color:#fff;background:radial-gradient(circle at 22% 18%,rgba(255,255,255,.32),transparent 26%),linear-gradient(135deg,#1469ff 0%,#0f8fff 48%,#16c7b7 100%);box-shadow:0 16px 36px rgba(20,105,255,.22)}
.brand-logo-cn svg{width:42px;height:auto}
.hero{margin-top:54px;max-width:880px}
h1{margin:0;max-width:820px;font-size:clamp(34px,3.8vw,54px);line-height:1.12;letter-spacing:-.045em}
.message{max-width:780px;margin:24px 0 0;color:var(--muted);font-size:clamp(15px,1.3vw,18px);line-height:1.7}
.next{display:grid;grid-template-columns:1fr auto;gap:18px;align-items:center;margin-top:34px;padding:15px 18px;border:1px solid var(--next-line);border-radius:18px;background:var(--next-bg);box-shadow:var(--next-shadow)}
.next p{margin:0;color:var(--next-text);font-size:clamp(14px,1.25vw,16px);font-weight:650;letter-spacing:-.02em}
code{padding:0;border-radius:0;background:transparent;color:var(--primary);font:850 clamp(14px,1.25vw,16px) ui-monospace,SFMono-Regular,Menlo,monospace;white-space:nowrap}
.safety{margin:24px 0 0;color:var(--muted);font-size:14px;line-height:1.65}
.is-error .next{display:none}
.agora-page{--ink:#f8fbff;--muted:#a9b8cd;--primary:#35d6a5;--line:rgba(255,255,255,.14);--panel:rgba(9,20,38,.54);--shadow:0 24px 90px rgba(0,0,0,.22);--next-line:rgba(255,255,255,.1);--next-bg:rgba(5,11,20,.72);--next-text:#dbe7ff;--next-shadow:none;--bg:radial-gradient(circle at 12% 12%,rgba(53,214,165,.28),transparent 30%),radial-gradient(circle at 88% 16%,rgba(122,167,255,.22),transparent 32%),radial-gradient(circle at 54% 100%,rgba(62,84,255,.18),transparent 36%),linear-gradient(150deg,#050b14 0%,#101b33 48%,#07111f 100%)}
.agora-page:before{opacity:.2;background-image:linear-gradient(rgba(255,255,255,.08) 1px,transparent 1px),linear-gradient(90deg,rgba(255,255,255,.08) 1px,transparent 1px);background-size:44px 44px;mask-image:radial-gradient(circle at 50% 48%,black,transparent 72%)}
.shengwang-page{--ink:#10233f;--muted:#5f6f85;--primary:#1469ff;--line:rgba(24,56,96,.14);--panel:linear-gradient(135deg,rgba(255,255,255,.96),rgba(255,255,255,.78)),radial-gradient(circle at 0% 0%,rgba(20,105,255,.08),transparent 34%);--shadow:0 30px 100px rgba(16,35,63,.16),0 1px 0 rgba(255,255,255,.78) inset;--next-line:rgba(24,56,96,.16);--next-bg:#f8fafc;--next-text:#5f6f85;--next-shadow:0 12px 40px rgba(20,61,112,.06);--bg:radial-gradient(circle at 18% 12%,rgba(20,105,255,.34),transparent 28%),radial-gradient(circle at 85% 78%,rgba(22,199,183,.24),transparent 30%),linear-gradient(135deg,#e8f1ff 0%,#f8fbff 42%,#e8fff9 100%)}
.shengwang-page:before{background:linear-gradient(rgba(10,24,48,.045) 1px,transparent 1px),linear-gradient(90deg,rgba(10,24,48,.045) 1px,transparent 1px),radial-gradient(circle at 50% 0%,rgba(5,8,18,.1),transparent 34%);background-size:44px 44px,44px 44px,auto;opacity:.8;mask-image:radial-gradient(circle at 50% 48%,black,transparent 78%)}
@media (max-width:900px){
  .page{padding-top:72px}
  .card{padding:28px}
  .next{grid-template-columns:1fr}
  code{white-space:normal;word-break:break-word}
}
`
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
