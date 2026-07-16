import requests

img_id = "3a608f55-d76e-fa96-d0b1-0789fbc48f1e"
url = f"https://www.artic.edu/iiif/2/{img_id}/full/843,/0/default.jpg"
headers = {
    "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
    "AIC-User-Agent": "SpiceWallpaper (spice@dixieflatline76.com)"
}
r2 = requests.get(url, headers=headers)
print("IIIF 843, headers:", r2.status_code)

url = f"https://www.artic.edu/iiif/2/{img_id}/full/!800,800/0/default.jpg"
r3 = requests.get(url, headers=headers)
print("IIIF !800,800 headers:", r3.status_code)

r4 = requests.get(url)
print("IIIF !800,800 NO headers:", r4.status_code)
