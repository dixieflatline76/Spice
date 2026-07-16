import re

with open('pkg/gallery/gallery.go', 'r', encoding='utf-8') as f:
    content = f.read()

# Replace CSS
css_old = """        .matboard {
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
        }"""

css_new = """        .matboard {
            background-color: #fdfdfa;
            padding: 25px;
            box-shadow: inset 0 0 10px rgba(0,0,0,0.5);
            display: inline-block;
        }

        .artwork {
            max-width: 100%;
            height: auto;
            max-height: 350px;
            display: block;
            box-shadow: 0 2px 5px rgba(0,0,0,0.4);
        }"""

content = content.replace(css_old, css_new)

# Replace HTML
html_old = """    <div class="gallery-container">
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
    </div>"""

html_new = """    <div class="gallery-container">
        <!-- Frame 1 -->
        {{if .Image1}}
        <div class="frame">
            <div class="matboard">
                <img class="artwork" src="{{.Image1}}" />
            </div>
        </div>
        {{end}}
        
        <!-- Frame 2 -->
        {{if .Image2}}
        <div class="frame">
            <div class="matboard">
                <img class="artwork" src="{{.Image2}}" />
            </div>
        </div>
        {{end}}

        <!-- Frame 3 (Centerpiece) -->
        {{if .Image3}}
        <div class="frame" style="transform: scale(1.1); z-index: 10;">
            <div class="matboard">
                <img class="artwork" src="{{.Image3}}" />
            </div>
        </div>
        {{end}}

        <!-- Frame 4 -->
        {{if .Image4}}
        <div class="frame">
            <div class="matboard">
                <img class="artwork" src="{{.Image4}}" />
            </div>
        </div>
        {{end}}

        <!-- Frame 5 -->
        {{if .Image5}}
        <div class="frame">
            <div class="matboard">
                <img class="artwork" src="{{.Image5}}" />
            </div>
        </div>
        {{end}}
    </div>"""

content = content.replace(html_old, html_new)

with open('pkg/gallery/gallery.go', 'w', encoding='utf-8') as f:
    f.write(content)
print("Updated gallery.go template!")
