package gallery

import (
	"bytes"
	"fmt"
	"html/template"
)

// GenerateHTML takes a collection title and up to 5 image URLs and returns
// a beautiful virtual gallery wall HTML string.
func GenerateHTML(title string, imageUrls []string) (string, error) {
	// Pad or truncate to exactly 5 frames for the layout
	var frames [5]string
	for i := 0; i < 5; i++ {
		if i < len(imageUrls) {
			frames[i] = imageUrls[i]
		} else {
			// Placeholder gradient if not enough images
			frames[i] = ""
		}
	}

	tmplStr := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}} - Spice Gallery</title>
    <style>
        body {
            background-color: #4a3b2c;
            margin: 0;
            padding: 60px 40px;
            min-height: 100vh;
            display: flex;
            flex-direction: column;
            justify-content: center;
            align-items: center;
            font-family: system-ui, -apple-system, sans-serif;
        }

        h1 {
            color: rgba(255, 255, 255, 0.9);
            font-weight: 300;
            letter-spacing: 2px;
            margin-bottom: 60px;
            text-shadow: 0 4px 10px rgba(0,0,0,0.5);
            text-align: center;
        }

        .gallery-container {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
            gap: 60px;
            max-width: 1400px;
            width: 100%;
            align-items: center;
        }

        .frame {
            background: linear-gradient(135deg, #4a2f1d, #2d1c11);
            padding: 12px;
            border-radius: 4px;
            box-shadow: 0 30px 60px rgba(0, 0, 0, 0.6), 0 10px 20px rgba(0, 0, 0, 0.4);
            transition: transform 0.3s ease;
            position: relative;
        }

        .frame:hover {
            transform: scale(1.02) translateY(-5px);
        }

        .matboard {
            background-color: #fdfdfa;
            padding: 35px;
            box-shadow: inset 0 0 10px rgba(0,0,0,0.5);
            display: flex;
            justify-content: center;
            align-items: center;
        }

        .artwork {
            width: 100%;
            height: auto;
            display: block;
            box-shadow: inset 0 0 15px rgba(0,0,0,0.3), 0 2px 5px rgba(0,0,0,0.2);
            background: linear-gradient(45deg, #8b7355, #cdb49a);
        }

        /* Staggered masonry offsets */
        .frame:nth-child(even) {
            transform: translateY(40px);
        }
        .frame:nth-child(even):hover {
            transform: scale(1.02) translateY(35px);
        }
    </style>
</head>
<body>
    <h1>{{.Title}}</h1>
    <div class="gallery-container">
        <!-- Frame 1 -->
        <div class="frame">
            <div class="matboard">
                <div class="artwork" style="aspect-ratio: 3/4; {{if .Image1}}background: url('{{.Image1}}') center/cover;{{end}}"></div>
            </div>
        </div>
        
        <!-- Frame 2 -->
        <div class="frame">
            <div class="matboard">
                <div class="artwork" style="aspect-ratio: 1/1; {{if .Image2}}background: url('{{.Image2}}') center/cover;{{end}}"></div>
            </div>
        </div>

        <!-- Frame 3 (Centerpiece) -->
        <div class="frame" style="transform: scale(1.1); z-index: 10;">
            <div class="matboard">
                <div class="artwork" style="aspect-ratio: 4/3; {{if .Image3}}background: url('{{.Image3}}') center/cover;{{end}}"></div>
            </div>
        </div>

        <!-- Frame 4 -->
        <div class="frame">
            <div class="matboard">
                <div class="artwork" style="aspect-ratio: 3/4; {{if .Image4}}background: url('{{.Image4}}') center/cover;{{end}}"></div>
            </div>
        </div>

        <!-- Frame 5 -->
        <div class="frame">
            <div class="matboard">
                <div class="artwork" style="aspect-ratio: 4/5; {{if .Image5}}background: url('{{.Image5}}') center/cover;{{end}}"></div>
            </div>
        </div>
    </div>
</body>
</html>`

	t, err := template.New("gallery").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse gallery template: %w", err)
	}

	data := struct {
		Title  string
		Image1 string
		Image2 string
		Image3 string
		Image4 string
		Image5 string
	}{
		Title:  title,
		Image1: frames[0],
		Image2: frames[1],
		Image3: frames[2],
		Image4: frames[3],
		Image5: frames[4],
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute gallery template: %w", err)
	}

	return buf.String(), nil
}
