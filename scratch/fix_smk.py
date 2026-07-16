import json

with open('docs/collections/smk.json', 'r', encoding='utf-8') as f:
    smk = json.load(f)

# smk collections currently look like:
# { "ids": [...], "key": "Best of SMK", "type": "curated" }

for c in smk.get('collections', []):
    old_key = c.get('key')
    if old_key == "Best of SMK":
        c['key'] = "smk_highlights"
        c['name'] = "⭐ Best of SMK"
        c['name_translations'] = {"en": "⭐ Best of SMK"}
    elif old_key == "Danish & Nordic Art (1750-1900)":
        c['key'] = "smk_danish_nordic"
        c['name'] = old_key
        c['name_translations'] = {"en": old_key}
    elif old_key == "European Old Masters (1300-1800)":
        c['key'] = "smk_european_old_masters"
        c['name'] = old_key
        c['name_translations'] = {"en": old_key}
    elif old_key == "French Art (1900-1930)":
        c['key'] = "smk_french_art"
        c['name'] = old_key
        c['name_translations'] = {"en": old_key}
    elif old_key == "Danish Modernism":
        c['key'] = "smk_danish_modernism"
        c['name'] = old_key
        c['name_translations'] = {"en": old_key}
    elif old_key == "Other Highlights":
        c['key'] = "smk_other_highlights"
        c['name'] = old_key
        c['name_translations'] = {"en": old_key}
    elif old_key == "Artistic Nudes":
        c['key'] = "smk_artistic_nudes"
        c['name'] = old_key
        c['name_translations'] = {"en": old_key}
    elif old_key:
        c['name'] = old_key
        c['key'] = old_key.lower().replace(" ", "_")
        c['name_translations'] = {"en": old_key}

smk['description'] = "Statens Museum for Kunst: Curated Collections"
smk['version'] = "v1.0.0"

with open('docs/collections/smk.json', 'w', encoding='utf-8') as f:
    json.dump(smk, f, indent=4, ensure_ascii=False)

print("SMK json fixed.")
