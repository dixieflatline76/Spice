package i18n

import (
	"embed"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"text/template"

	"fyne.io/fyne/v2/lang"
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
	// 1. Register with Fyne for standard system-default behavior
	if err := lang.AddTranslationsFS(translationFS, "translations"); err != nil {
		_ = err
	}

	// 2. Load into our local maps for manual override support
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

// T returns the localized version of the given English string.
func T(english string) string {
	mu.RLock()
	code := currentLanguage
	mu.RUnlock()

	if code != "" {
		if m, ok := translations[code]; ok {
			if val, ok := m[english]; ok {
				return strings.TrimSpace(val)
			}
		}
	}
	return strings.TrimSpace(lang.Localize(english))
}

// Tf returns the localized version of the given English template string.
func Tf(english string, data any) string {
	mu.RLock()
	code := currentLanguage
	mu.RUnlock()

	if code != "" {
		if m, ok := translations[code]; ok {
			if val, ok := m[english]; ok {
				return strings.TrimSpace(applyTemplate(val, data))
			}
		}
	}
	return strings.TrimSpace(lang.Localize(english, data))
}

// N returns the localized plural form of the given English string.
func N(english string, count int, data ...any) string {
	// Fyne's lang package handles plural rules.
	// For now, if we have a manual override, we'll try to use Fyne's plural logic
	// by passing the right data, but Fyne's LocalizePlural doesn't take a locale.
	// Since Spice mostly uses T and Tf, and N is rare, we'll fall back to lang.LocalizePlural.
	return strings.TrimSpace(lang.LocalizePlural(english, count, data...))
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
