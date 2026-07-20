#!/usr/bin/env python3
import urllib.request
import re
import json
import sys
import argparse
import time

def parse_getty(url, retries=3, delay=1.0):
    match = re.search(r'/object/([A-Za-z0-9]+)', url)
    if not match:
        print(f"Could not extract object slug from URL: {url}", file=sys.stderr)
        return None
    
    slug = match.group(1)
    
    # First, fetch the HTML page to find the UUID
    html_url = f"https://www.getty.edu/art/collection/object/{slug}"
    req_html = urllib.request.Request(html_url, headers={'User-Agent': 'Mozilla/5.0'})
    
    html_data = None
    for attempt in range(retries):
        try:
            with urllib.request.urlopen(req_html) as response:
                html_data = response.read().decode('utf-8')
            break
        except Exception as e:
            print(f"Attempt {attempt + 1} failed for HTML {url}: {e}", file=sys.stderr)
            if attempt < retries - 1:
                time.sleep(delay)
                
    if not html_data:
        print(f"Failed to fetch HTML for {url}", file=sys.stderr)
        return None

    # Extract UUID from script tag
    script_match = re.search(r'<script id=\'local_id_manager\' type=\'application/json\'>(.*?)</script>', html_data)
    if not script_match:
        print(f"Could not find local_id_manager script for {url}", file=sys.stderr)
        return None

    try:
        id_data = json.loads(script_match.group(1))
        indexed_id = id_data.get('indexedId', '')
        if not indexed_id.startswith('object/'):
            print(f"Invalid indexedId format for {url}", file=sys.stderr)
            return None
        uuid = indexed_id.replace('object/', '')
    except Exception as e:
        print(f"Failed to parse local_id_manager for {url}: {e}", file=sys.stderr)
        return None

    api_url = f"https://data.getty.edu/museum/collection/object/{uuid}"
    req_api = urllib.request.Request(api_url, headers={'User-Agent': 'Mozilla/5.0', 'Accept': 'application/ld+json'})
    
    data = None
    for attempt in range(retries):
        try:
            with urllib.request.urlopen(req_api) as response:
                data = json.loads(response.read().decode('utf-8'))
            break
        except Exception as e:
            print(f"Attempt {attempt + 1} failed for API {api_url}: {e}", file=sys.stderr)
            if attempt < retries - 1:
                time.sleep(delay)

    if not data:
        print(f"Failed to fetch JSON-LD for {uuid}", file=sys.stderr)
        return None

    # Parse JSON-LD simple fields
    title = data.get('_label', 'Unknown Title')
    artist = 'Unknown Artist'
    
    try:
        if 'produced_by' in data and 'carried_out_by' in data['produced_by']:
            carried_out = data['produced_by']['carried_out_by']
            if len(carried_out) > 0:
                artist = carried_out[0].get('_label', 'Unknown Artist')
    except Exception:
        pass

    return {
        "id": uuid,
        "title": title,
        "artist": artist,
        "url": html_url
    }

def main():
    parser = argparse.ArgumentParser(description="Parse Getty object URLs.")
    parser.add_argument("urls", nargs="+", help="One or more Getty object URLs")
    args = parser.parse_args()

    items = []
    for i, url in enumerate(args.urls):
        if i > 0:
            time.sleep(1.0)
        item = parse_getty(url)
        if item:
            items.append(item)

    if len(items) == 1:
        print(json.dumps(items[0], indent=4))
    else:
        print(json.dumps(items, indent=4))

if __name__ == '__main__':
    main()
