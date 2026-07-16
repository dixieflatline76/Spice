import requests

url = "https://www.artic.edu/iiif/2/8c322744-93ff-ef72-1323-e57579bc79de/full/843,/0/default.jpg"
r = requests.get(url)
print("No headers, 843,: ", r.status_code)
