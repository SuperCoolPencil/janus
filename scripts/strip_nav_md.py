#!/usr/bin/env python3
"""Strip the Sphinx-generated 'Quick search' / 'Contents' block from Markdown files.
Keeps README.md unchanged.
Usage: python scripts/strip_nav_md.py --dir docs/ext4
"""
import argparse
from pathlib import Path


def strip_nav(text: str) -> str:
    # Remove from '### Quick search' up to and including the '### This Page' section
    start_marker = '### Quick search'
    end_marker = '### This Page'
    si = text.find(start_marker)
    if si == -1:
        return text
    ei = text.find(end_marker, si)
    if ei == -1:
        # if no end marker, try to remove until first top-level heading after start
        # find next line that starts with '# '
        rest = text[si:]
        import re
        m = re.search(r"^# ", rest, flags=re.M)
        if m:
            ei = si + m.start()
        else:
            # fallback: remove until 2nd occurrence of '\n\n#' or stop
            return text
    # remove block and return
    # include the end_marker line and following content up to next blank line
    # find end of line after end_marker
    end_line = text.find('\n', ei)
    if end_line == -1:
        end_line = ei
    new_text = text[:si] + text[end_line+1:]
    return new_text


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--dir', required=True)
    args = parser.parse_args()

    root = Path(args.dir)
    md_files = list(root.rglob('*.md'))
    for md in md_files:
        if md.name.lower() == 'readme.md':
            continue
        text = md.read_text(encoding='utf-8')
        new = strip_nav(text)
        if new != text:
            md.write_text(new, encoding='utf-8')
            print('Stripped', md)

if __name__ == '__main__':
    main()
