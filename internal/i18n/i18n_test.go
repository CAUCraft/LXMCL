package i18n

import (
	"errors"
	"regexp"
	"slices"
	"testing"
)

func TestCatalogsUseMatchingKeysAndPlaceholders(t *testing.T) {
	// Catalogs currently use ordered %d, %q, %s, %v and %w substitutions.
	verbs := regexp.MustCompile("%[%dqsvw]")
	base, en := catalogs[defaultLocale], catalogs["en"]
	for key, msg := range base {
		if msg == "" || en[key] == "" {
			t.Errorf("empty or missing message: %s", key)
		}
		if !slices.Equal(verbs.FindAllString(msg, -1), verbs.FindAllString(en[key], -1)) {
			t.Errorf("format arguments differ for %s: %q and %q", key, msg, en[key])
		}
	}
	for key := range en {
		if _, ok := base[key]; !ok {
			t.Errorf("English key %s is missing from the default locale", key)
		}
	}
}

func TestLocaleAndMissingMessageFallback(t *testing.T) {
	key := "progress.download_and_verify"
	for _, tc := range []struct{ locale, want string }{
		{"", "下载并校验 client.jar"},
		{"zh-CN", "下载并校验 client.jar"},
		{" EN_us ", "Downloading and verifying client.jar"},
		{"unsupported", "下载并校验 client.jar"},
	} {
		if got := selectCatalog(tc.locale, catalogs).text(key, "client.jar"); got != tc.want {
			t.Errorf("locale %q rendered %q; want %q", tc.locale, got, tc.want)
		}
	}
	partial := catalog{messages: map[string]string{}, fallback: catalogs[defaultLocale]}
	if got := partial.text(key, "client.jar"); got != "下载并校验 client.jar" {
		t.Errorf("missing translation did not fall back: %q", got)
	}
	if got := partial.text("missing.key"); got != "[missing.key]" {
		t.Errorf("missing message key was not identified: %q", got)
	}
}

func TestLocalizedErrorPreservesBothCauses(t *testing.T) {
	retry := errors.New("retry")
	cause := errors.New("unreadable")
	err := selectCatalog("en", catalogs).err("common.wrapped_errors", retry, cause)
	if err.Error() != "retry: unreadable" || !errors.Is(err, retry) || !errors.Is(err, cause) {
		t.Fatalf("error text or chain changed: %v", err)
	}
}
