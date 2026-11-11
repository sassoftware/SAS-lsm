set -e
rm -f toc.json
node build.js G.docx >/dev/null
soffice --headless --convert-to pdf G.docx >/dev/null 2>&1
node -e "const L=require('./lib');require('./content1')();require('./content2')();require('./content3')();require('./content4')();console.log(JSON.stringify(L.HEADINGS))" > heads.json
python3 - <<'PY'
import subprocess, json, re
n=int(re.search(r'Pages:\s+(\d+)',subprocess.run(['pdfinfo','G.pdf'],capture_output=True,text=True).stdout).group(1))
pages=[[l.strip() for l in subprocess.run(['pdftotext','-layout','-f',str(p),'-l',str(p),'G.pdf','-'],capture_output=True,text=True).stdout.split('\n')] for p in range(1,n+1)]
hs=json.load(open('heads.json'))
res=[];start=2
for lvl,t in hs:
    found=None
    for p in range(start,n):
        if t in pages[p]: found=p; break
    if found is None: print('NOT FOUND',t); found=start
    res.append(found+1); start=found
json.dump(res,open('toc.json','w')); print(res)
PY
node build.js G.docx >/dev/null
soffice --headless --convert-to pdf G.docx >/dev/null 2>&1
pdfinfo G.pdf | grep Pages
