import requests

url = "https://www.artic.edu/iiif/2/8c322744-93ff-ef72-1323-e57579bc79de/full/!800,800/0/default.jpg"
headers = {
    "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
    "Referer": "https://www.artic.edu/"
}
r = requests.get(url, headers=headers)
print("With headers:", r.status_code)
