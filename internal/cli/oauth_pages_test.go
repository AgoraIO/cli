package cli

import (
	"strings"
	"testing"
)

func TestOAuthCallbackSuccessPageUsesFinalGlobalDesign(t *testing.T) {
	html := renderOAuthCallbackSuccessPage(callbackPageConfig{Region: regionGlobal})

	for _, want := range []string{
		`<html lang="en">`,
		"agora-page",
		"Agora%20Logo%20Crisp.webp",
		"You are now authenticated with Agora CLI",
		"This browser step completed successfully. The CLI can now use this account as your active local configuration.",
		"agora auth status",
		"You can close this window and return to your terminal.",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("expected global login page to contain %q", want)
		}
	}

	for _, forbidden := range []string{"agora whoami", "global-minimal", "global-deck", "shengwang", "声网", "brand-logo-cn"} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("global login page should not contain legacy variant content %q", forbidden)
		}
	}
}

func TestOAuthCallbackSuccessPageDefaultsToGlobalBranding(t *testing.T) {
	html := renderOAuthCallbackSuccessPage(callbackPageConfig{Region: "test"})

	for _, want := range []string{
		"agora-page",
		"Agora%20Logo%20Crisp.webp",
		"You are now authenticated with Agora CLI",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("expected fallback login page to contain %q", want)
		}
	}

	for _, forbidden := range []string{"shengwang", "声网", "brand-logo-cn"} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("fallback login page should not contain China branding %q", forbidden)
		}
	}
}

func TestOAuthCallbackSuccessPageUsesFinalChinaDesign(t *testing.T) {
	html := renderOAuthCallbackSuccessPage(callbackPageConfig{Region: regionCN})

	for _, want := range []string{
		`<html lang="zh-CN">`,
		"shengwang-page",
		"brand-logo-cn",
		"你已成功登录声网 CLI",
		"此浏览器步骤已完成。CLI 现在可以将此账号作为当前本地配置继续使用。",
		"agora auth status",
		"你可以关闭此页面，回到终端继续操作。",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("expected China login page to contain %q", want)
		}
	}

	for _, forbidden := range []string{"agora whoami", "cn-console", "cn-silk"} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("China login page should not contain legacy variant content %q", forbidden)
		}
	}
}

func TestOAuthCallbackErrorPageKeepsRegionBranding(t *testing.T) {
	html := renderOAuthCallbackErrorPage(callbackPageConfig{Region: regionCN}, "Login state mismatch", "Return to the terminal and restart login.")

	for _, want := range []string{
		"shengwang-page is-error",
		"Login state mismatch",
		"Return to the terminal and restart login.",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("expected error login page to contain %q", want)
		}
	}

	if strings.Contains(html, "agora auth status") {
		t.Fatal("error login page should not show success action command")
	}
}
