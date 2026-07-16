import json
with open('scratch/met_titles.json', 'r', encoding='utf-8') as f:
    items = json.load(f)

print('Possible missed nudes:')
for i in items:
    t = i['title'].lower()
    if any(word in t for word in ['woman', 'female', 'bath', 'diana', 'nymph', 'eve', 'susanna', 'dana', 'leda', 'cupid', 'apollo']):
        print(f"{i['id']}: {i['title']}")
