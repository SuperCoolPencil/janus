#!/usr/bin/env python3
"""Verify markdown links in a directory point to existing files.
Usage: python scripts/verify_md_links.py --dir docs/ext4
"""
import argparse
from pathlib import Path
import re

MD_LINK_RE = re.compile(r"\[([^\]]+)\]\(([^)]+)\)")


def is_external(link: str) -> bool:
    return link.startswith('http://') or link.startswith('https://') or link.startswith('#')


def normalize_link(path, link):
    # Remove anchors
    link = link.split('#')[0]
    # Remove query
    link = link.split('?')[0]
    return (path.parent / link).resolve()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--dir', required=True)
    args = parser.parse_args()

    root = Path(args.dir)
    md_files = list(root.rglob('*.md'))
    broken = []
    for md in md_files:
        text = md.read_text(encoding='utf-8')
        for m in MD_LINK_RE.finditer(text):
            link = m.group(2).strip()
            if is_external(link):
                continue
            # ignore mailto:
            if link.startswith('mailto:'):
                continue
            norm = normalize_link(md, link)
            # If link had no extension, try .md and .html
            if norm.exists():
                continue
            if not norm.suffix:
                if (norm.with_suffix('.md')).exists() or (norm.with_suffix('.html')).exists():
                    continue
            broken.append((md, link))

    if not broken:
        print('No broken links found in', root)
        return 0

    print('Broken links:')
    for md, link in broken:
        print(f'- {md}: {link}')
    return 1


if __name__ == '__main__':
    raise SystemExit(main())
