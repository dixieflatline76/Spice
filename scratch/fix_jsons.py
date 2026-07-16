import json

# Fix artic.json
with open('docs/collections/artic.json', 'r', encoding='utf-8') as f:
    artic = json.load(f)
for c in artic['collections']:
    if c['key'] == 'artic_impressionism':
        c['name'] = 'Impressionism & Post-Impressionism'
        c['name_translations'] = {
            'en': 'Impressionism & Post-Impressionism',
            'fr': 'Impressionnisme et postimpressionnisme',
            'zh': '印象派与后印象派'
        }
    if c['key'] == 'artic_highlights':
        c['name'] = '⭐ Best of AIC'
with open('docs/collections/artic.json', 'w', encoding='utf-8') as f:
    json.dump(artic, f, indent=4, ensure_ascii=False)

# Fix cleveland.json
with open('docs/collections/cleveland.json', 'r', encoding='utf-8') as f:
    cma = json.load(f)
for c in cma['collections']:
    if c['key'] == 'artistic_nudes':
        if "143547" in c['ids']:
            c['ids'].remove("143547")
with open('docs/collections/cleveland.json', 'w', encoding='utf-8') as f:
    json.dump(cma, f, indent=4, ensure_ascii=False)

# Fix getty.json
with open('docs/collections/getty.json', 'r', encoding='utf-8') as f:
    getty = json.load(f)
for c in getty['collections']:
    if c['key'] == 'getty_nudes_vintage':
        if "ca9023dd-235f-4344-ac1e-73e5e5f44ceb" in c['ids']:
            c['ids'].remove("ca9023dd-235f-4344-ac1e-73e5e5f44ceb")
        if "cca6ce55-2381-4a92-b572-bbc5def38ef2" in c['ids']:
            c['ids'].remove("cca6ce55-2381-4a92-b572-bbc5def38ef2")
with open('docs/collections/getty.json', 'w', encoding='utf-8') as f:
    json.dump(getty, f, indent=4, ensure_ascii=False)

print("JSON fixes applied.")
