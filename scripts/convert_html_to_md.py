#!/usr/bin/env python3
"""
Convert HTML files under a directory to Markdown, updating links from .html to .md.
Requires: pip install markdownify beautifulsoup4 lxml
Usage: python scripts/convert_html_to_md.py --dir docs/ext4 --out-dir docs/ext4
"""
import argparse
import os
from pathlib import Path
from bs4 import BeautifulSoup

try:
    from markdownify import markdownify as md
except Exception:
    md = None


def convert_html_file(src_path: Path, dest_path: Path):
    html = src_path.read_text(encoding='utf-8')
    soup = BeautifulSoup(html, 'lxml')
    # Prefer content inside <body>
    body = soup.body
    if body is None:
        body_html = soup.prettify()
    else:
        body_html = ''.join(str(ch) for ch in body.children)

    if md:
        md_text = md(body_html, heading_style='ATX')
    else:
        # Fallback: strip tags very simply
        md_text = body.get_text("\n") if body is not None else soup.get_text("\n")

    # Convert links ending with .html to .md
    md_text = md_text.replace('.html)', '.md)')
    md_text = md_text.replace('.html#', '.md#')
    md_text = md_text.replace('.html"', '.md"')

    # Add a title at top if available
    title = None
    if soup.title and soup.title.string:
        title = soup.title.string.strip()
    elif soup.find(['h1', 'h2']):
        title = soup.find(['h1','h2']).get_text().strip()

    out_lines = []
    if title:
        out_lines.append(f"# {title}\n")
    out_lines.append(md_text)

    dest_path.parent.mkdir(parents=True, exist_ok=True)
    dest_path.write_text('\n'.join(out_lines), encoding='utf-8')


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--dir', required=True, help='Source directory containing HTML files')
    parser.add_argument('--out-dir', default=None, help='Output directory for Markdown files (defaults to source dir)')
    parser.add_argument('--ext', default='.html', help='Source file extension to convert')
    parser.add_argument('--dry-run', action='store_true')
    args = parser.parse_args()

    src_dir = Path(args.dir)
    if args.out_dir:
        out_dir = Path(args.out_dir)
    else:
        out_dir = src_dir

    html_files = sorted(src_dir.rglob(f'*{args.ext}'))
    if not html_files:
        print('No HTML files found in', src_dir)
        return

    print(f'Found {len(html_files)} files; output -> {out_dir} (dry-run={args.dry_run})')

    for f in html_files:
        rel = f.relative_to(src_dir)
        dest = out_dir / rel.with_suffix('.md')
        print(f'Converting {f} -> {dest}')
        if not args.dry_run:
            convert_html_file(f, dest)

    print('Done.')


if __name__ == '__main__':
    main()
