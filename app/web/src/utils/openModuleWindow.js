export function openModuleWindow(path, name = 'zhigu-module') {
  const width = Math.min(1440, Math.max(1024, window.screen.availWidth - 80))
  const height = Math.min(960, Math.max(720, window.screen.availHeight - 80))
  const features = `popup=yes,width=${width},height=${height},noopener=yes,noreferrer=yes`
  const win = window.open(path, name, features)
  if (win) {
    try { win.opener = null } catch (_) { /* browser policy may block assignment */ }
  }
  return win
}

export function openIntelWindow() {
  return openModuleWindow('/app/intel', 'zhigu-intel')
}
