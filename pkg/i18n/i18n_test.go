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

func TestGetTranslationsForKeys(t *testing.T) {
	keys := []string{"Preferences", "Active"}
	res := GetTranslationsForKeys(keys)

	if len(res) == 0 {
		t.Fatal("Expected translations map to be populated")
	}

	if dePrefs, ok := res["de"]["Preferences"]; !ok || dePrefs == "" {
		t.Errorf("Expected 'de' translation for 'Preferences', got %v", dePrefs)
	}

	if frLang, ok := res["fr"]["Active"]; !ok || frLang == "" {
		t.Errorf("Expected 'fr' translation for 'Active', got %v", frLang)
	}
}
