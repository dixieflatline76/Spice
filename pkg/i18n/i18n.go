package i18n

import (
	"embed"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"text/template"

	golocale "github.com/jeandeaual/go-locale"
)

//go:generate go run ../../cmd/util/gen_i18n/main.go
//go:embed translations
var translationFS embed.FS

var (
	translations    = make(map[string]map[string]string)
	currentLanguage string
	mu              sync.RWMutex
)

func init() {
	loadLocalTranslations()
}

func loadLocalTranslations() {
	entries, err := translationFS.ReadDir("translations")
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		langCode := strings.TrimSuffix(entry.Name(), ".json")
		f, err := translationFS.Open("translations/" + entry.Name())
		if err != nil {
			continue
		}
		data, err := io.ReadAll(f)
		_ = f.Close()
		if err != nil {
			continue
		}

		var m map[string]string
		if err := json.Unmarshal(data, &m); err == nil {
			mu.Lock()
			translations[langCode] = m
			mu.Unlock()
		}
	}
}

func detectOSLocale() string {
	loc, err := golocale.GetLocale()
	if err != nil || loc == "" {
		return "en"
	}
	return loc
}

// mapLocaleToCode matches raw OS locale codes (e.g. "en_US", "zh-TW", "de-DE") to supported app language codes.
func mapLocaleToCode(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if strings.HasPrefix(raw, "zh-hant") || strings.HasPrefix(raw, "zh_hant") ||
		strings.HasPrefix(raw, "zh-tw") || strings.HasPrefix(raw, "zh_tw") ||
		strings.HasPrefix(raw, "zh-hk") || strings.HasPrefix(raw, "zh_hk") {
		return "zh-Hant"
	}
	if strings.HasPrefix(raw, "zh") {
		return "zh"
	}
	for _, sl := range SupportedLanguages {
		if strings.HasPrefix(raw, strings.ToLower(sl.Code)) {
			return sl.Code
		}
	}
	return "en"
}

func getEffectiveLanguage() string {
	mu.RLock()
	code := currentLanguage
	mu.RUnlock()

	if code == "" {
		code = mapLocaleToCode(detectOSLocale())
	}
	return code
}

// SetLanguage sets the application-wide language.
// Supported: "English", "Deutsch" or codes "en", "de".
// Empty string or "System Default" reverts to system locale.
func SetLanguage(lang string) {
	mu.Lock()
	defer mu.Unlock()

	lang = strings.ToLower(lang)
	if lang == "" || lang == "system default" {
		currentLanguage = ""
		return
	}

	for _, sl := range SupportedLanguages {
		if strings.ToLower(sl.Name) == lang || strings.ToLower(sl.Code) == lang {
			currentLanguage = sl.Code
			return
		}
	}

	currentLanguage = "" // Default if not found
}

// GetLanguage returns the current application language code.
// Returns an empty string if set to "System Default".
func GetLanguage() string {
	mu.RLock()
	defer mu.RUnlock()
	return currentLanguage
}

// T returns the localized version of the given English string.
func T(english string) string {
	code := getEffectiveLanguage()
	if code != "" {
		if m, ok := translations[code]; ok {
			if val, ok := m[english]; ok {
				return strings.TrimSpace(val)
			}
		}
	}
	return strings.TrimSpace(english)
}

// TMap returns the localized version from the provided translation map,
// falling back to standard T() if not found.
func TMap(english string, trans map[string]string) string {
	code := getEffectiveLanguage()
	if code != "" && trans != nil {
		if val, ok := trans[code]; ok {
			return strings.TrimSpace(val)
		}
		// Fallbacks for Chinese locales from JSON files
		if code == "zh" {
			if val, ok := trans["zh-CN"]; ok {
				return strings.TrimSpace(val)
			}
		}
		if code == "zh-Hant" {
			if val, ok := trans["zh-TW"]; ok {
				return strings.TrimSpace(val)
			}
		}
	}
	return T(english)
}

// Tf returns the localized version of the given English template string.
func Tf(english string, data any) string {
	code := getEffectiveLanguage()
	tmplStr := english
	if code != "" {
		if m, ok := translations[code]; ok {
			if val, ok := m[english]; ok {
				tmplStr = val
			}
		}
	}
	return strings.TrimSpace(applyTemplate(tmplStr, data))
}

// N returns the localized plural form of the given English string.
func N(english string, count int, data ...any) string {
	if len(data) > 0 {
		return Tf(english, data[0])
	}
	return T(english)
}

// GetTranslationsForKeys returns a map of all language codes to their translations for a subset of keys.
func GetTranslationsForKeys(keys []string) map[string]map[string]string {
	mu.RLock()
	defer mu.RUnlock()

	res := make(map[string]map[string]string)
	for code, langMap := range translations {
		langRes := make(map[string]string)
		for _, k := range keys {
			if val, ok := langMap[k]; ok {
				langRes[k] = strings.TrimSpace(val)
			}
		}
		if len(langRes) > 0 {
			res[code] = langRes
		}
	}
	return res
}

func applyTemplate(tmplStr string, data any) string {
	// Simple template application mirroring Fyne's behavior
	t, err := template.New("i18n").Parse(tmplStr)
	if err != nil {
		return tmplStr
	}

	var buf strings.Builder
	if err := t.Execute(&buf, data); err != nil {
		return tmplStr
	}
	return buf.String()
}
