import json

with open(r'docs\collections\metmuseum.json', 'r', encoding='utf-8') as f:
    data = json.load(f)

nude_ids = []
photo_ids = []

for c in data['collections']:
    if c['key'] == 'metmuseum_artistic_nudes':
        nude_ids = c['ids']
    elif c['key'] == 'metmuseum_vintage_photography_nudes':
        photo_ids = c['ids']

combined_nudes = nude_ids + photo_ids

# Rebuild collections without the photo nudes category, and update artistic nudes
new_collections = []
for c in data['collections']:
    if c['key'] == 'metmuseum_vintage_photography_nudes':
        continue
    if c['key'] == 'metmuseum_artistic_nudes':
        c['ids'] = combined_nudes
    new_collections.append(c)

data['collections'] = new_collections

with open(r'docs\collections\metmuseum.json', 'w', encoding='utf-8') as f:
    json.dump(data, f, indent=4, ensure_ascii=False)

print("Merged vintage photography nudes into artistic nudes.")
