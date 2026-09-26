import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { stat } from 'node:fs/promises';

const here = path.dirname(fileURLToPath(import.meta.url));
const require = createRequire(path.resolve(here, '../../../app/web/package.json'));
const { chromium } = require('playwright');
const root = path.resolve(here, '..');
const htmlPath = path.join(here, 'product-intro.html');
const videoPath = path.join(root, 'zhigu-product-intro-60s.webm');
const times = [0, 5.8, 12, 22, 32, 42, 48.5, 55, 59.9];

const browser = await chromium.launch({ headless: true });
const page = await browser.newPage({ viewport: { width: 1280, height: 720 } });
const errors = [];
page.on('console', (msg) => { if (msg.type() === 'error') errors.push(msg.text()); });
page.on('pageerror', (error) => errors.push(error.message));

const sceneChecks = [];
for (const time of times) {
  await page.goto(`${pathToFileURL(htmlPath).href}?time=${time}`);
  await page.waitForFunction(() => window.__zhiguReady === true);
  await page.waitForTimeout(80);
  sceneChecks.push(await page.evaluate((t) => {
    const active = document.querySelector('.scene.active');
    const images = [...active.querySelectorAll('img')];
    return {
      time: t,
      activeId: active.id,
      images: images.map((img) => ({ src: img.getAttribute('src'), complete: img.complete, naturalWidth: img.naturalWidth })),
      activeBox: active.getBoundingClientRect().toJSON(),
    };
  }, time));
}

await page.goto(pathToFileURL(videoPath).href);
await page.waitForSelector('video');
await page.waitForFunction(() => {
  const video = document.querySelector('video');
  return video && Number.isFinite(video.duration) && video.duration > 0;
}, null, { timeout: 15000 });
const video = await page.evaluate(() => {
  const v = document.querySelector('video');
  return { duration: v.duration, width: v.videoWidth, height: v.videoHeight };
});
const info = await stat(videoPath);
await browser.close();

const failures = [];
if (errors.length) failures.push(...errors);
if (video.duration < 50 || video.duration > 70) failures.push(`duration out of bounds: ${video.duration}`);
if (video.width !== 1280 || video.height !== 720) failures.push(`unexpected dimensions: ${video.width}x${video.height}`);
if (info.size >= 100 * 1024 * 1024) failures.push(`file too large: ${info.size}`);
for (const check of sceneChecks) {
  if (check.images.some((img) => !img.complete || img.naturalWidth === 0)) failures.push(`image failed at ${check.time}s`);
  if (check.activeBox.width !== 1280 || check.activeBox.height !== 720) failures.push(`scene bounds failed at ${check.time}s`);
}

console.log(JSON.stringify({ video, bytes: info.size, sceneChecks, failures }, null, 2));
if (failures.length) process.exit(1);
