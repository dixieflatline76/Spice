package rijksmuseum

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"
)

func TestResolveNudesScript(t *testing.T) {
	accessions := []string{"SK-A-3841", "SK-C-1351", "SK-A-26"}

	for _, acc := range accessions {
		url := fmt.Sprintf("https://www.rijksmuseum.nl/api/en/collection/%s?key=fpzb0PBj&format=json", acc)
		resp, err := http.Get(url)
		if err != nil {
			t.Logf("Failed: %v", err)
			continue
		}
		defer resp.Body.Close()

		b, _ := ioutil.ReadAll(resp.Body)
		var result map[string]interface{}
		json.Unmarshal(b, &result)

		artObject, ok := result["artObject"].(map[string]interface{})
		if !ok {
			t.Logf("No artObject for %s. Response: %s", acc, string(b))
			continue
		}

		id := artObject["id"].(string) // maybe not numeric?
		title := artObject["title"].(string)
		artist := artObject["principalMaker"].(string)
		webImage := artObject["webImage"].(map[string]interface{})
		imgURL := webImage["url"].(string) // this is probably not iiif, but it will do!

		t.Logf("ID: %s", id)
		t.Logf("Title: %s", title)
		t.Logf("Artist: %s", artist)
		t.Logf("URL: %s", imgURL)
	}
}
