import requests

r = requests.get("https://api.artic.edu/api/v1/artworks/14655")
data = r.json()
img_id = data['data']['image_id']
print("Image ID:", img_id)

url = f"https://www.artic.edu/iiif/2/{img_id}/full/843,/0/default.jpg"
r2 = requests.get(url)
print("IIIF 843,:", r2.status_code)

url = f"https://www.artic.edu/iiif/2/{img_id}/full/!800,800/0/default.jpg"
r3 = requests.get(url)
print("IIIF !800,800:", r3.status_code)
