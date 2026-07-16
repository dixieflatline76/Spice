import re
import json

with open(r'C:\Users\karlk\Downloads\bookmarks_7_16_26.html', 'r', encoding='utf-8') as f:
    content = f.read()

match = re.search(r'>MET Collection</H3>\s*<DL><p>(.*?)</DL><p>', content, re.IGNORECASE | re.DOTALL)
if not match:
    print('Folder not found')
    exit(1)

block = match.group(1)
links = re.findall(r'<A HREF="([^"]+)"[^>]*>(.*?)</A>', block, re.IGNORECASE)

items = []
for url, title in links:
    m = re.search(r'search/(\d+)', url)
    if m:
        items.append({
            'id': m.group(1),
            'title': title
        })

with open('scratch/met_titles.json', 'w', encoding='utf-8') as f:
    json.dump(items, f, indent=2)

print(f"Saved {len(items)} items to scratch/met_titles.json")
