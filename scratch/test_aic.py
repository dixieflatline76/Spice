import requests

url = "https://www.artic.edu/iiif/2/8c322744-93ff-ef72-1323-e57579bc79de/full/!800,800/0/default.jpg"
r = requests.get(url)
print(r.status_code)
print(r.headers.get("content-type"))
