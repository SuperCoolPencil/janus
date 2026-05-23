#!/usr/bin/env python3
"""Remove Sphinx footer lines from markdown files under a directory, except README.md."""
import argparse
from pathlib import Path

REMOVE_PATTERNS = [
    'Show Source',
    'Powered by',
    'Page source',
    '©The kernel development community',
    '© The kernel development community',
]


def should_remove_line(line: str) -> bool:
    for p in REMOVE_PATTERNS:
        if p in line:
            return True
    return False


def clean_file(path: Path):
    text = path.read_text(encoding='utf-8')
    lines = text.splitlines()
    new_lines = [ln for ln in lines if not should_remove_line(ln)]
    # remove excessive trailing blank lines
    while len(new_lines) > 1 and new_lines[-1].strip() == '' and new_lines[-2].strip() == '':
        new_lines.pop(-1)
    new_text = '\n'.join(new_lines) + '\n'
    if new_text != text:
        path.write_text(new_text, encoding='utf-8')
        print('Cleaned', path)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--dir', required=True)
    args = parser.parse_args()
    root = Path(args.dir)
    for md in root.rglob('*.md'):
        if md.name.lower() == 'readme.md':
            continue
        clean_file(md)

if __name__ == '__main__':
    main()
