import json

with open('scratch/met_categories.json', 'r', encoding='utf-8') as f:
    cats = json.load(f)

# The correct nudes:
nudes = ["204758", "437826", "436002", "459093", "436918", "14521"]
vintage_photo_nudes = ["282051", "266982"]

cats["metmuseum_artistic_nudes"] = nudes
cats["metmuseum_vintage_photography_nudes"] = vintage_photo_nudes

# Remove nudes from other categories (especially best_of and european_paintings)
for cat in cats:
    if cat not in ["metmuseum_artistic_nudes", "metmuseum_vintage_photography_nudes"]:
        cats[cat] = [x for x in cats[cat] if x not in nudes and x not in vintage_photo_nudes]

cats = {k: v for k, v in cats.items() if len(v) > 0}

labels = {
    "metmuseum_best_of": {
        "en": "Metropolitan Masterpieces",
        "fr": "Chefs-d'œuvre du Met",
        "zh": "大都会名作"
    },
    "metmuseum_arms_and_armor": {
        "en": "Arms and Armor",
        "fr": "Armes et Armures",
        "zh": "武器与盔甲"
    },
    "metmuseum_antiquities": {
        "en": "Antiquities",
        "fr": "Antiquités",
        "zh": "古物"
    },
    "metmuseum_asian_art": {
        "en": "Asian Art",
        "fr": "Art Asiatique",
        "zh": "亚洲艺术"
    },
    "metmuseum_american_art": {
        "en": "American Art",
        "fr": "Art Américain",
        "zh": "美国艺术"
    },
    "metmuseum_european_paintings": {
        "en": "European Paintings",
        "fr": "Peintures Européennes",
        "zh": "欧洲绘画"
    },
    "metmuseum_vintage_photography_nudes": {
        "en": "Vintage Photography Nudes",
        "fr": "Nus en Photographie Vintage",
        "zh": "复古裸体摄影"
    },
    "metmuseum_artistic_nudes": {
        "en": "Artistic Nudes",
        "fr": "Nus Artistiques",
        "zh": "艺术裸体"
    }
}

ordered_keys = [
    "metmuseum_best_of",
    "metmuseum_european_paintings",
    "metmuseum_american_art",
    "metmuseum_asian_art",
    "metmuseum_antiquities",
    "metmuseum_arms_and_armor",
    "metmuseum_vintage_photography_nudes",
    "metmuseum_artistic_nudes"
]

collections = []
for k in ordered_keys:
    if k in cats:
        collections.append({
            "key": k,
            "type": "curated",
            "name": labels[k]["en"],
            "name_translations": labels[k],
            "ids": cats[k]
        })

out_json = {
    "description": "Metropolitan Museum of Art: Curated Collections",
    "version": "v1.0.0",
    "collections": collections
}

with open(r'docs\collections\metmuseum.json', 'w', encoding='utf-8') as f:
    json.dump(out_json, f, indent=4, ensure_ascii=False)

print("Updated docs/collections/metmuseum.json with correct schema")
