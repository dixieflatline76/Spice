package i18n

import (
	"testing"
)

func TestTranslationsLoaded(t *testing.T) {
	langs := []string{"en", "de", "fr", "es", "it", "pt", "zh"}

	for _, lang := range langs {
		SetLanguage(lang)
		// Check a known key that should exist in all
		translated := T("Preferences")
		if translated == "" || translated == "Preferences" && lang != "en" {
			t.Errorf("Expected translation for 'Preferences' in %s, got: %s", lang, translated)
		}
	}
}

func TestSetLanguage(t *testing.T) {
	SetLanguage("Deutsch")
	if currentLanguage != "de" {
		t.Errorf("Expected currentLanguage to be 'de', got %s", currentLanguage)
	}

	SetLanguage("Français")
	if currentLanguage != "fr" {
		t.Errorf("Expected currentLanguage to be 'fr', got %s", currentLanguage)
	}

	SetLanguage("简体中文")
	if currentLanguage != "zh" {
		t.Errorf("Expected currentLanguage to be 'zh', got %s", currentLanguage)
	}

	SetLanguage("System Default")
	if currentLanguage != "" {
		t.Errorf("Expected currentLanguage to be empty for System Default, got %s", currentLanguage)
	}
}
