import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';
import { mkdir, rm, stat } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const require = createRequire(path.resolve(here, '../../../app/web/package.json'));
const { chromium } = require('playwright');
const root = path.resolve(here, '..');
const htmlPath = path.join(here, 'product-intro.html');
const output = path.join(root, 'zhigu-product-intro-60s.webm');
const videoTemp = path.join(root, '.video-temp');

await rm(output, { force: true });
await rm(videoTemp, { recursive: true, force: true });
await mkdir(videoTemp, { recursive: true });

const browser = await chromium.launch({ headless: true });
const context = await browser.newContext({
  viewport: { width: 1280, height: 720 },
  deviceScaleFactor: 1,
  recordVideo: { dir: videoTemp, size: { width: 1280, height: 720 } },
});
const page = await context.newPage();
await page.goto(`${pathToFileURL(htmlPath).href}?mode=manual`);
await page.waitForFunction(() => window.__zhiguReady === true);
await page.evaluate(() => window.__zhiguStart());
await page.waitForTimeout(59030);
const video = page.video();
if (!video) throw new Error('Playwright did not create a video');
await page.close();
await video.saveAs(output);
await context.close();
await browser.close();
await rm(videoTemp, { recursive: true, force: true });

const info = await stat(output);
console.log(JSON.stringify({ output, bytes: info.size }, null, 2));
