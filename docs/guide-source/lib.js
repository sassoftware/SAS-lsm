// Helpers for building the SAS_lsm guide with docx-js.
const d = require('docx');
const { Paragraph, TextRun, Table, TableRow, TableCell, AlignmentType, BorderStyle,
        WidthType, ShadingType, HeadingLevel, PageBreak } = d;

const BLUE = '2E74B5';
const MONO = 'Courier New';

// inline markup: **bold**, `code`, *italic*
function runs(text, base = {}) {
  const out = [];
  const re = /(\*\*.+?\*\*|`.+?`|\*[^*\s][^*]*?\*)/g;
  let last = 0, m;
  while ((m = re.exec(text)) !== null) {
    if (m.index > last) out.push(new TextRun({ text: text.slice(last, m.index), ...base }));
    const t = m[0];
    if (t.startsWith('**')) out.push(new TextRun({ text: t.slice(2, -2), bold: true, ...base }));
    else if (t.startsWith('`')) out.push(new TextRun({ text: t.slice(1, -1), font: MONO, bold: true, size: 19, ...base }));
    else out.push(new TextRun({ text: t.slice(1, -1), italics: true, ...base }));
    last = m.index + t.length;
  }
  if (last < text.length) out.push(new TextRun({ text: text.slice(last), ...base }));
  return out;
}

const HEADINGS = [];
const H1 = t => (HEADINGS.push([1, t.toUpperCase()]), new Paragraph({ heading: HeadingLevel.HEADING_1, children: [new TextRun(t.toUpperCase())] }));
const H2 = t => (HEADINGS.push([2, t]), new Paragraph({ heading: HeadingLevel.HEADING_2, children: [new TextRun(t)] }));
const H3 = t => new Paragraph({ heading: HeadingLevel.HEADING_3, children: [new TextRun(t)] });
const P = (t, opts = {}) => new Paragraph({ spacing: { after: 140 }, ...opts, children: runs(t) });
const Note = t => new Paragraph({ spacing: { after: 140 }, children: [new TextRun({ text: 'Note: ', bold: true }), ...runs(t)] });
const Important = t => new Paragraph({
  spacing: { after: 160 },
  border: { left: { style: BorderStyle.SINGLE, size: 18, color: BLUE, space: 8 } },
  indent: { left: 200 },
  children: [new TextRun({ text: 'Important: ', bold: true, color: BLUE }), ...runs(t)],
});

const Bullet = (t, level = 0) => new Paragraph({ numbering: { reference: 'bullets', level }, spacing: { after: 60 }, children: runs(t) });

let listCounter = 0;
const numberedRefs = [];
function Numbered(items) {
  const ref = 'num' + (++listCounter);
  numberedRefs.push(ref);
  return items.map(t => new Paragraph({ numbering: { reference: ref, level: 0 }, spacing: { after: 80 }, children: runs(t) }));
}
function NumberedCont(items) {
  const ref = 'num' + listCounter;
  return items.map(t => new Paragraph({ numbering: { reference: ref, level: 0 }, spacing: { after: 80 }, children: runs(t) }));
}

function Code(text) {
  const lines = text.replace(/\n$/, '').split('\n');
  return lines.map((l, i) => new Paragraph({
    keepNext: i < lines.length - 1,
    keepLines: true,
    spacing: { before: i === 0 ? 60 : 0, after: i === lines.length - 1 ? 180 : 0, line: 240 },
    indent: { left: 360 },
    shading: { type: ShadingType.CLEAR, fill: 'F2F2F2', color: 'auto' },
    children: [new TextRun({ text: l === '' ? ' ' : l, font: MONO, size: 17 })],
  }));
}

const border = { style: BorderStyle.SINGLE, size: 4, color: '000000' };
const borders = { top: border, bottom: border, left: border, right: border };

// rows: array of arrays of strings. First row is the header.
// opts.mono: column indexes rendered in bold Courier (a leading * opts out for one cell)
function Tbl(rows, widths, opts = {}) {
  const total = widths.reduce((a, b) => a + b, 0);
  const mono = new Set(opts.mono || []);
  return new Table({
    width: { size: total, type: WidthType.DXA },
    columnWidths: widths,
    rows: rows.map((r, ri) => new TableRow({
      tableHeader: ri === 0,
      cantSplit: true,
      children: r.map((c, ci) => new TableCell({
        borders,
        width: { size: widths[ci], type: WidthType.DXA },
        margins: { top: 50, bottom: 50, left: 100, right: 100 },
        shading: ri === 0 ? { type: ShadingType.CLEAR, fill: 'DEEAF6', color: 'auto' } : undefined,
        children: String(c).split('\n').map(line => new Paragraph({
          alignment: ri === 0 ? AlignmentType.CENTER : AlignmentType.LEFT,
          children: ri === 0
            ? [new TextRun({ text: line, bold: true })]
            : (mono.has(ci) && !line.startsWith('*')
                ? [new TextRun({ text: line, font: MONO, bold: true, size: 18 })]
                : runs(line.startsWith('*') && mono.has(ci) ? line.slice(1) : line)),
        })),
      })),
    })),
  });
}
const Gap = () => new Paragraph({ spacing: { after: 120 }, children: [] });
const Break = () => new Paragraph({ children: [new PageBreak()] });

module.exports = { HEADINGS, d, runs, H1, H2, H3, P, Note, Important, Bullet, Numbered, NumberedCont, numberedRefs, Code, Tbl, Gap, Break, BLUE, MONO };
