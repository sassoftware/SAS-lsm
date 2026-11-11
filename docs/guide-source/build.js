const fs = require('fs');
const L = require('./lib');
const { Document, Packer, Paragraph, TextRun, AlignmentType, LevelFormat, Footer, Header, PageNumber,
        PageBreak, TabStopType, Tab } = L.d;

const body = [
  ...require('./content1')(),
  ...require('./content2')(),
  ...require('./content3')(),
  ...require('./content4')(),
];

// Static table of contents. Page numbers come from toc.json, produced by
// rendering the document once and locating each heading (see mk.sh).
function tocParas() {
  const pages = fs.existsSync('toc.json') ? JSON.parse(fs.readFileSync('toc.json')) : [];
  const out = [new Paragraph({ spacing: { after: 240 }, children: [new TextRun({ text: 'Contents', bold: true, size: 32, color: L.BLUE })] })];
  L.HEADINGS.forEach(([lvl, t], i) => out.push(new Paragraph({
    tabStops: [{ type: TabStopType.RIGHT, position: 9360, leader: 'dot' }],
    indent: { left: lvl === 1 ? 0 : 360 },
    spacing: { before: lvl === 1 ? 100 : 0, after: 20 },
    children: [new TextRun({ text: t, bold: lvl === 1, size: lvl === 1 ? 21 : 20 }),
               new TextRun({ children: [new Tab(), String(pages[i] || '00')], size: lvl === 1 ? 21 : 20 })],
  })));
  return out;
}

const title = [
  new Paragraph({ spacing: { before: 2400, after: 240 }, alignment: AlignmentType.CENTER,
    children: [new TextRun({ text: 'The SAS®9 Local Services Management Utility (SAS_lsm)', bold: true, size: 40, color: L.BLUE })] }),
  new Paragraph({ alignment: AlignmentType.CENTER, spacing: { after: 120 },
    children: [new TextRun({ text: 'Version 5', size: 32, bold: true })] }),
  new Paragraph({ alignment: AlignmentType.CENTER, spacing: { after: 1200 },
    children: [new TextRun({ text: 'Installation, Configuration, and Usage Guide', size: 28 })] }),
  new Paragraph({ alignment: AlignmentType.CENTER, spacing: { after: 120 },
    children: [new TextRun({ text: 'DRAFT FOR TESTING - build 5.0.0-dev.3', bold: true, size: 24, color: 'C00000' })] }),
  new Paragraph({ alignment: AlignmentType.CENTER,
    children: [new TextRun({ text: 'Applies to SAS 9.4 deployments on UNIX and Linux', italics: true })] }),
  new Paragraph({ children: [new PageBreak()] }),
  ...tocParas(),
  new Paragraph({ children: [new PageBreak()] }),
];

const numberingConfig = [
  { reference: 'bullets', levels: [
    { level: 0, format: LevelFormat.BULLET, text: '•', alignment: AlignmentType.LEFT, style: { paragraph: { indent: { left: 720, hanging: 360 } } } },
    { level: 1, format: LevelFormat.BULLET, text: 'o', alignment: AlignmentType.LEFT, style: { paragraph: { indent: { left: 1440, hanging: 360 } } } },
  ] },
  ...L.numberedRefs.map(ref => ({ reference: ref, levels: [
    { level: 0, format: LevelFormat.DECIMAL, text: '%1.', alignment: AlignmentType.LEFT, style: { paragraph: { indent: { left: 720, hanging: 360 } } } },
  ] })),
];

const doc = new Document({
  creator: 'Andy Foreman',
  title: 'SAS_lsm 5 Installation, Configuration, and Usage Guide',
  styles: {
    default: { document: { run: { font: 'Calibri', size: 22 } } },
    paragraphStyles: [
      { id: 'Heading1', name: 'Heading 1', basedOn: 'Normal', next: 'Normal', quickFormat: true,
        run: { size: 30, color: L.BLUE, font: 'Calibri Light' },
        paragraph: { spacing: { before: 360, after: 160 }, keepNext: true, outlineLevel: 0 } },
      { id: 'Heading2', name: 'Heading 2', basedOn: 'Normal', next: 'Normal', quickFormat: true,
        run: { size: 26, bold: true, color: L.BLUE, font: 'Calibri Light' },
        paragraph: { spacing: { before: 280, after: 120 }, keepNext: true, outlineLevel: 1 } },
      { id: 'Heading3', name: 'Heading 3', basedOn: 'Normal', next: 'Normal', quickFormat: true,
        run: { size: 23, bold: true, italics: true, color: '1F4E79', font: 'Calibri Light' },
        paragraph: { spacing: { before: 220, after: 100 }, keepNext: true, outlineLevel: 2 } },
    ],
  },
  numbering: { config: numberingConfig },
  sections: [{
    properties: { page: { size: { width: 12240, height: 15840 }, margin: { top: 1440, right: 1440, bottom: 1440, left: 1440 } } },
    headers: { default: new Header({ children: [new Paragraph({ alignment: AlignmentType.RIGHT,
      children: [new TextRun({ text: 'SAS_lsm 5 Installation and Usage Guide - DRAFT', size: 16, color: '808080' })] })] }) },
    footers: { default: new Footer({ children: [new Paragraph({ alignment: AlignmentType.CENTER,
      children: [new TextRun({ children: [PageNumber.CURRENT] })] })] }) },
    children: [...title, ...body].flat(Infinity),
  }],
});

Packer.toBuffer(doc).then(b => { fs.writeFileSync(process.argv[2] || 'guide.docx', b); console.log('written'); });
