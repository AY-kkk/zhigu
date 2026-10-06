import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';
import { mkdir } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const require = createRequire(path.resolve(here, '../../../app/web/package.json'));
const { chromium } = require('playwright');
const root = path.resolve(here, '..');
const htmlPath = path.join(here, 'product-intro.html');
const qaDir = path.join(root, 'qa');
const times = [0.5, 8, 16, 27, 36, 45, 51, 57];
await mkdir(qaDir, { recursive: true });
const browser = await chromium.launch({ headless: true });
const page = await browser.newPage({ viewport: { width: 1280, height: 720 }, deviceScaleFactor: 1 });
for (const time of times) {
  await page.goto(`${pathToFileURL(htmlPath).href}?time=${time}`);
  await page.waitForFunction(() => window.__zhiguReady === true);
  await page.waitForTimeout(1100);
  await page.screenshot({ path: path.join(qaDir, `frame-${String(time).replace('.', '-')}.png`) });
}
await browser.close();
console.log(JSON.stringify({ qaDir, times }, null, 2));
