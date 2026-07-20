#!/usr/bin/env python3
import urllib.request
import re
import json
import sys
import argparse
import time

def parse_smk(url, retries=3, delay=1.0):
    # Extract object_number from URL (e.g., https://open.smk.dk/artwork/image/KMS4753)
    match = re.search(r'artwork/(?:image|object)/([^/?]+)', url)
    if not match:
        match = re.search(r'object_number=([^&]+)', url)
        if not match:
            print(f"Could not extract object number from URL: {url}", file=sys.stderr)
            return None
    
    object_number = match.group(1)
    api_url = f"https://api.smk.dk/api/v1/art/?object_number={object_number}"
    req = urllib.request.Request(api_url, headers={'User-Agent': 'Mozilla/5.0'})
    
    data = None
    for attempt in range(retries):
        try:
            with urllib.request.urlopen(req) as response:
                data = json.loads(response.read().decode('utf-8'))
            break
        except Exception as e:
            print(f"Attempt {attempt + 1} failed for {url}: {e}", file=sys.stderr)
            if attempt < retries - 1:
                time.sleep(delay)
                
    if not data or 'items' not in data or len(data['items']) == 0:
        print(f"Failed to fetch {url} after {retries} attempts.", file=sys.stderr)
        return None

    item = data['items'][0]
    
    # Extract title
    title = "Unknown"
    if 'titles' in item and len(item['titles']) > 0:
        title = item['titles'][0].get('title', 'Unknown')
        for t in item['titles']:
            if t.get('language') in ['en', 'engelsk']:
                title = t.get('title')
                break

    # Extract artist
    artist = "Unknown"
    if 'artist' in item and len(item['artist']) > 0:
        artist = item['artist'][0]

    return {
        "id": object_number,
        "title": title,
        "artist": artist,
        "url": url
    }

def main():
    parser = argparse.ArgumentParser(description="Parse Statens Museum for Kunst object URLs.")
    parser.add_argument("urls", nargs="+", help="One or more SMK object URLs")
    args = parser.parse_args()

    items = []
    for i, url in enumerate(args.urls):
        if i > 0:
            time.sleep(1.0) # pacing
        item = parse_smk(url)
        if item:
            items.append(item)

    if len(items) == 1:
        print(json.dumps(items[0], indent=4))
    else:
        print(json.dumps(items, indent=4))

if __name__ == '__main__':
    main()
