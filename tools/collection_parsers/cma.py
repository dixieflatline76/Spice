#!/usr/bin/env python3
import urllib.request
import re
import json
import sys
import argparse
import time

def parse_cma(url, retries=3, delay=1.0):
    match = re.search(r'/art/([A-Za-z0-9\.]+)', url)
    if not match:
        print(f"Could not extract object ID from URL: {url}", file=sys.stderr)
        return None
    
    object_id = match.group(1)
    api_url = f"https://openaccess-api.clevelandart.org/api/artworks/{object_id}"
    req = urllib.request.Request(api_url, headers={'User-Agent': 'Mozilla/5.0'})
    
    data = None
    for attempt in range(retries):
        try:
            with urllib.request.urlopen(req) as response:
                resp_json = json.loads(response.read().decode('utf-8'))
                data = resp_json.get('data', {})
            break
        except Exception as e:
            print(f"Attempt {attempt + 1} failed for {url}: {e}", file=sys.stderr)
            if attempt < retries - 1:
                time.sleep(delay)
                
    if not data:
        print(f"Failed to fetch {url} after {retries} attempts.", file=sys.stderr)
        return None

    artist = "Unknown Artist"
    if 'creators' in data and len(data['creators']) > 0:
        artist = data['creators'][0].get('description', 'Unknown Artist')

    return {
        "id": str(data.get('id', object_id)),
        "title": data.get('title', ''),
        "artist": artist,
        "department": data.get('department', ''),
        "type": data.get('type', ''),
        "url": url
    }

def main():
    parser = argparse.ArgumentParser(description="Parse CMA object URLs.")
    parser.add_argument("urls", nargs="+", help="One or more CMA object URLs")
    args = parser.parse_args()

    items = []
    for i, url in enumerate(args.urls):
        if i > 0:
            time.sleep(1.0) # pacing
        item = parse_cma(url)
        if item:
            items.append(item)

    if len(items) == 1:
        print(json.dumps(items[0], indent=4))
    else:
        print(json.dumps(items, indent=4))

if __name__ == '__main__':
    main()
