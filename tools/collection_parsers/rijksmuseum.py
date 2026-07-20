#!/usr/bin/env python3
import urllib.request
import re
import json
import sys
import argparse

import time

def parse_rijks(url, retries=3, delay=1.0):
    req = urllib.request.Request(url, headers={'User-Agent': 'Mozilla/5.0'})
    
    html = None
    for attempt in range(retries):
        try:
            with urllib.request.urlopen(req) as response:
                html = response.read().decode('utf-8')
            break  # Success
        except Exception as e:
            print(f"Attempt {attempt + 1} failed for {url}: {e}", file=sys.stderr)
            if attempt < retries - 1:
                time.sleep(delay)
    
    if not html:
        print(f"Failed to fetch {url} after {retries} attempts.", file=sys.stderr)
        return None

    title = ''
    match = re.search(r'<meta property="og:title" content="(.*?)"', html)
    if match:
        title = match.group(1)

    image_url = ''
    match = re.search(r'<meta property="og:image" content="(.*?)"', html)
    if match:
        image_url = match.group(1)
        # Convert preview webp/jpg IIIF URLs into full high-res max URLs
        image_url = re.sub(r'(iiif\.micr\.io/[^/]+)/.*?/default\.(?:webp|jpg)', r'\1/full/max/0/default.jpg', image_url)

    numeric_id = ''
    match = re.search(r'"https://id\.rijksmuseum\.nl/(\d+)"', html)
    if match:
        numeric_id = match.group(1)

    accession_id = ''
    # Look for SK-A-1234 or similar object number formats
    match = re.search(r'(SK-[A-Z]-\d+)', html)
    if match:
        accession_id = match.group(1)

    artist = ''
    # The artist is usually embedded in the subtitle paragraph text
    match = re.search(r'<p class="body object-subtitle".*?<!--\[-->(.*?),', html)
    if match:
        artist = match.group(1).strip()

    return {
        "accession_id": accession_id,
        "artist": artist,
        "id": numeric_id,
        "image_url": image_url,
        "title": title,
        "url": url
    }

def main():
    parser = argparse.ArgumentParser(description="Parse Rijksmuseum object URLs into Spice JSON collection items.")
    parser.add_argument("urls", nargs="+", help="One or more Rijksmuseum object URLs")
    args = parser.parse_args()

    items = []
    for i, url in enumerate(args.urls):
        if i > 0:
            time.sleep(1.0) # Respect rate limits between requests
        item = parse_rijks(url)
        if item:
            items.append(item)

    if len(items) == 1:
        # Output exactly in the requested format if single URL
        print(json.dumps(items[0], indent=4))
    else:
        # Array format for multiple
        print(json.dumps(items, indent=4))

if __name__ == '__main__':
    main()
