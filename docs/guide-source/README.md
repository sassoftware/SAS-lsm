# Install and Usage Guide source

The Word and PDF guides in `docs/` are generated from these files with docx-js.

```sh
npm install docx            # once
bash mk.sh                  # builds G.docx and G.pdf (needs LibreOffice, pdftotext, pdfinfo)
```

`mk.sh` builds the document twice: the first PDF is used to find the page number of each heading, and those page numbers are written into the table of contents.
