package gallery

import (
	"strings"
	"testing"
)

func TestGenerateHTMLWithTranslations(t *testing.T) {
	items := []GalleryItem{
		{ID: "1", URL: "http://example.com/1.jpg"},
	}

	titleTranslations := map[string]string{
		"de": "Das Beste aus Cleveland",
		"fr": "Le Meilleur de Cleveland",
	}

	html, err := GenerateHTML("Best of Cleveland", titleTranslations, items, 1.5, true)
	if err != nil {
		t.Fatalf("GenerateHTML failed: %v", err)
	}

	if !strings.Contains(html, `const translations = {`) {
		t.Error("Expected generated HTML to contain Javascript translations dictionary")
	}

	if !strings.Contains(html, `"Das Beste aus Cleveland"`) {
		t.Error("Expected generated HTML to contain German translated title in dictionary")
	}

	if !strings.Contains(html, `data-i18n="Best of Cleveland"`) {
		t.Error("Expected title tag to have data-i18n attribute")
	}

	if !strings.Contains(html, `data-i18n="Back to Index"`) {
		t.Error("Expected Back to Index link to have data-i18n attribute")
	}
}
