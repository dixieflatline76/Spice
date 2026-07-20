#!/usr/bin/env python3
import urllib.request
import re
import json
import sys
import argparse
import time

def parse_npm(url, retries=3, delay=1.0):
    match = re.search(r'cid=(\d+)', url)
    if not match:
        match = re.search(r'id=(\d+)', url)
        if not match:
            print(f"Could not extract object ID from URL: {url}", file=sys.stderr)
            return None
    
    object_id = match.group(1)
    api_url = f"https://digitalarchive.npm.gov.tw/Integrate/GetJson?cid={object_id}&dept=U"
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
                
    if not data or 'label' not in data:
        print(f"Failed to fetch valid manifest for {url} after {retries} attempts.", file=sys.stderr)
        return None

    title = data.get('label', 'Unknown')
    # Clean up standard prefix if present
    if title.startswith('NPM \u6545\u5bae - '):
        title = title.replace('NPM \u6545\u5bae - ', '')

    return {
        "id": object_id,
        "title": title,
        "artist": "Unknown",
        "url": url
    }

def main():
    parser = argparse.ArgumentParser(description="Parse National Palace Museum (NPM) object URLs.")
    parser.add_argument("urls", nargs="+", help="One or more NPM object URLs")
    args = parser.parse_args()

    items = []
    for i, url in enumerate(args.urls):
        if i > 0:
            time.sleep(1.0) # pacing
        item = parse_npm(url)
        if item:
            items.append(item)

    if len(items) == 1:
        print(json.dumps(items[0], indent=4))
    else:
        print(json.dumps(items, indent=4))

if __name__ == '__main__':
    main()
