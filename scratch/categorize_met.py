import json
import re

with open('scratch/met_titles.json', 'r', encoding='utf-8') as f:
    items = json.load(f)

# Categories
best_of = []
nudes = []
nude_photos = []
armor = []
antiquities = []
asian_art = []
american_art = []
european_paintings = []

for item in items:
    title = item['title'].lower()
    id_str = item['id']
    
    # Keyword detection
    is_nude = any(x in title for x in ['nude', 'venus', 'bather', 'odalisque', 'medusa', 'adam and eve', 'perseus'])
    is_photo = any(x in title for x in ['photograph', 'daguerreotype', 'albumen', 'gelatin', 'cyanotype', 'salt print'])
    is_armor = any(x in title for x in ['armor', 'helmet', 'sword', 'saber', 'scabbard', 'spur', 'revolver', 'colt', 'rapier'])
    is_antiquity = any(x in title for x in ['egyptian', 'roman', 'greek', 'funerary mask', 'statuette', 'iron age', 'halaf', 'tolita', 'moche'])
    is_asian = any(x in title for x in ['japan', 'china', 'korea', 'edo', 'tang', 'ming', 'qing', 'hokusai', 'shunsho', 'buddha', 'bodhisattva', 'nepal', 'javanese'])
    is_american = any(x in title for x in ['american', 'winslow homer', 'bierstadt', 'thomas cole', 'eakins', 'sargent', 'chase', 'kensett', 'leutze'])
    is_euro_painting = any(x in title for x in ['van gogh', 'rembrandt', 'vermeer', 'monet', 'renoir', 'cezanne', 'courbet', 'david', 'klimt', 'titian', 'bellini', 'canaletto', 'ruisdael', 'la tour', 'lawrence'])

    # Assign to categories
    if is_nude:
        if is_photo:
            nude_photos.append(id_str)
        else:
            nudes.append(id_str)
            
    if is_armor:
        armor.append(id_str)
        
    if is_antiquity:
        antiquities.append(id_str)
        
    if is_asian:
        asian_art.append(id_str)
        
    if is_american:
        american_art.append(id_str)
        
    if is_euro_painting:
        european_paintings.append(id_str)

    # Best of logic: mix of everything, no nudes, no violence (subjective, but we exclude armor to be safe, maybe include 1 or 2 famous armors? user said "no violence", armor is weapons/violence)
    # Exclude nudes
    if not is_nude and not is_armor:
        best_of.append(id_str)

# Combine and output
out = {
    "metmuseum_best_of": best_of[:30], # Just take top 30 mixed items
    "metmuseum_arms_and_armor": armor,
    "metmuseum_antiquities": antiquities,
    "metmuseum_asian_art": asian_art,
    "metmuseum_american_art": american_art,
    "metmuseum_european_paintings": european_paintings,
    "metmuseum_artistic_nudes": nudes,
    "metmuseum_vintage_photography_nudes": nude_photos
}

with open('scratch/met_categories.json', 'w', encoding='utf-8') as f:
    json.dump(out, f, indent=2)

print("Categorized items:")
for k, v in out.items():
    print(f"{k}: {len(v)} items")
