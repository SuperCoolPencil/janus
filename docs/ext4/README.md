# ext4 docs (Markdown)

This README explains how to convert the HTML docs in this folder to Markdown and update internal links.

Conversion script (added at `scripts/convert_html_to_md.py`) can be used to perform the conversion.

Quick steps:

1. Install dependencies:

```powershell
python -m pip install --upgrade pip
pip install markdownify beautifulsoup4 lxml
```

2. Run the converter (dry run first):

```powershell
python ..\..\scripts\convert_html_to_md.py --dir . --dry-run
```

3. To actually write Markdown files in place (overwriting or alongside HTML):

```powershell
python ..\..\scripts\convert_html_to_md.py --dir .
```

Notes:
- The script will write `.md` files next to the `.html` files by default.
- Links that point to `*.html` will be rewritten to `*.md` where possible.
- For best results, install `markdownify` so headings and structure are preserved.
