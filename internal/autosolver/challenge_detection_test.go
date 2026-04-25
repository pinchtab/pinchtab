package autosolver

import "testing"

func TestDetectChallengeIntent_Turnstile(t *testing.T) {
	intent := DetectChallengeIntent(
		"Just a moment...",
		"https://example.com/cdn-cgi/challenge-platform/h/b/orchestrate/chl_page/v1",
		`<script src="https://challenges.cloudflare.com/turnstile/v0/api.js"></script>`,
	)
	if intent == nil {
		t.Fatal("expected challenge intent")
	}
	if intent.Type != IntentCaptcha {
		t.Fatalf("expected captcha intent, got %q", intent.Type)
	}
	if intent.ChallengeType != "turnstile" {
		t.Fatalf("expected turnstile challenge type, got %q", intent.ChallengeType)
	}
}

func TestDetectChallengeIntent_RecaptchaV2(t *testing.T) {
	intent := DetectChallengeIntent(
		"Verify",
		"https://example.com/login",
		`<div class="g-recaptcha" data-sitekey="abc"></div>
		 <script src="https://www.google.com/recaptcha/api.js"></script>`,
	)
	if intent == nil {
		t.Fatal("expected challenge intent")
	}
	if intent.ChallengeType != "recaptcha-v2" {
		t.Fatalf("expected recaptcha-v2 challenge type, got %q", intent.ChallengeType)
	}
}

func TestDetectChallengeIntent_RecaptchaV3(t *testing.T) {
	intent := DetectChallengeIntent(
		"Welcome",
		"https://example.com/secure",
		`<script src="https://www.google.com/recaptcha/api.js?render=site_key"></script>
		 <script>grecaptcha.execute('site_key', {action: 'login'})</script>`,
	)
	if intent == nil {
		t.Fatal("expected challenge intent")
	}
	if intent.ChallengeType != "recaptcha-v3" {
		t.Fatalf("expected recaptcha-v3 challenge type, got %q", intent.ChallengeType)
	}
}

func TestDetectChallengeIntent_HCaptcha(t *testing.T) {
	intent := DetectChallengeIntent(
		"Verify",
		"https://example.com/verify",
		`<div class="h-captcha" data-sitekey="abc"></div>
		 <script src="https://hcaptcha.com/1/api.js" async defer></script>`,
	)
	if intent == nil {
		t.Fatal("expected challenge intent")
	}
	if intent.ChallengeType != "hcaptcha" {
		t.Fatalf("expected hcaptcha challenge type, got %q", intent.ChallengeType)
	}
}

func TestDetectChallengeIntent_CustomJS(t *testing.T) {
	intent := DetectChallengeIntent(
		"Browser Integrity Check",
		"https://example.com/challenge",
		`<html><body><script>window._cf_chl_opt = {};</script><div>Please enable JavaScript</div></body></html>`,
	)
	if intent == nil {
		t.Fatal("expected challenge intent")
	}
	if intent.ChallengeType != "custom-js" {
		t.Fatalf("expected custom-js challenge type, got %q", intent.ChallengeType)
	}
	if intent.Type != IntentBlocked {
		t.Fatalf("expected blocked intent for custom-js, got %q", intent.Type)
	}
}

func TestDetectChallengeIntent_None(t *testing.T) {
	intent := DetectChallengeIntent(
		"Example Domain",
		"https://example.com",
		`<html><body><h1>Example Domain</h1></body></html>`,
	)
	if intent != nil {
		t.Fatalf("expected nil intent, got %+v", intent)
	}
}
