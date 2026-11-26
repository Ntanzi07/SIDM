// Confere a acessibilidade das telas rodando, com o axe-core num Chrome sem janela.
//
// Uso (com o backend e o frontend no ar):
//   set SIDM_A11Y_EMAIL=voce@splice.com.br
//   set SIDM_A11Y_PASSWORD=...            (nunca no código nem no git)
//   npm run a11y                           todas as telas, tema claro e escuro
//   npm run a11y -- /configurar?aba=conta  só as telas passadas
//
// Ou num arquivo frontend/a11y.env.local (ver abaixo). Opcionais: SIDM_URL (padrão http://localhost:5173), SIDM_A11Y_PACKAGE (um pacote
// para conferir também as telas dele) e CHROME_PATH (usar outro Chrome/Edge).
// Sai com código 1 quando acha problema, para poder rodar em CI.
import { existsSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import puppeteer from 'puppeteer'
import { AxePuppeteer } from '@axe-core/puppeteer'

// As variáveis também podem ficar em frontend/a11y.env.local (fora do git pelo
// *.local), uma por linha: SIDM_A11Y_EMAIL=..., SIDM_A11Y_PASSWORD=...
const ENV_FILE = fileURLToPath(new URL('../a11y.env.local', import.meta.url))
if (existsSync(ENV_FILE)) process.loadEnvFile(ENV_FILE)

// sem CHROME_PATH e sem o Chrome do puppeteer baixado: usa o Edge do Windows
const EDGE = 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'
const CHROME = process.env.CHROME_PATH || (process.platform === 'win32' && existsSync(EDGE) ? EDGE : undefined)

const BASE = (process.env.SIDM_URL || 'http://localhost:5173').replace(/\/$/, '')
const EMAIL = process.env.SIDM_A11Y_EMAIL
const PASSWORD = process.env.SIDM_A11Y_PASSWORD
const PACKAGE = process.env.SIDM_A11Y_PACKAGE

// as regras da WCAG 2.2 nível AA (o que se cobra de um sistema web)
const TAGS = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa']
const THEMES = [['light', 'claro'], ['dark', 'escuro']]

const PAGES = [
  '/pacotes',
  '/pacotes/criar',
  '/processar',
  '/tarefas',
  '/exportar',
  '/dashboard',
  '/configurar?aba=conta',
  '/configurar?aba=atalhos',
  '/configurar?aba=perfis-anotacao',
  '/configurar?aba=perfis-import',
  '/configurar?aba=equipamentos',
  '/configurar?aba=cameras',
  '/configurar?aba=usuarios',
  '/configurar?aba=sistema',
]
if (PACKAGE) {
  const p = encodeURIComponent(PACKAGE)
  PAGES.push(`/pacotes/${p}`, `/pacotes/${p}/anotacao`, `/pacotes/${p}/verificadas`)
}

const asked = process.argv.slice(2)
const pages = asked.length ? asked : PAGES
if (!EMAIL || !PASSWORD) {
  console.error('Defina SIDM_A11Y_EMAIL e SIDM_A11Y_PASSWORD (de um usuário admin, para ver todas as abas), no ambiente ou em frontend/a11y.env.local.')
  process.exit(2)
}

const wait = (ms) => new Promise((r) => setTimeout(r, ms))
const IMPACT = { critical: 'crítico', serious: 'sério', moderate: 'moderado', minor: 'leve' }
const ORDER = ['critical', 'serious', 'moderate', 'minor']

const browser = await puppeteer.launch({
  headless: true,
  executablePath: CHROME,
  args: process.getuid?.() === 0 ? ['--no-sandbox'] : [], // root (Docker/Linux) precisa
  defaultViewport: { width: 1366, height: 900 },
})
const page = await browser.newPage()

async function open(path) {
  await page.goto(BASE + path, { waitUntil: 'domcontentloaded', timeout: 30000 })
  // a tela carrega os dados depois: espera o conteúdo e um respiro para as listas
  await page.waitForSelector('main, form', { timeout: 15000 }).catch(() => {})
  await wait(1500)
}

async function check(path, theme) {
  const r = await new AxePuppeteer(page).withTags(TAGS).analyze()
  return r.violations.map((v) => ({ ...v, path, theme }))
}

const found = []
let failed = 0
try {
  // a tela de login é a única vista sem conta
  for (const [theme, label] of THEMES) {
    await page.goto(BASE + '/login', { waitUntil: 'domcontentloaded' })
    await page.evaluate((t) => localStorage.setItem('sidm.theme', t), theme)
    await open('/login')
    const v = await check('/login', label)
    found.push(...v)
    console.log(`${v.length ? '✗' : '✓'} /login (${label})${v.length ? `: ${v.length} regra(s)` : ''}`)
  }

  await open('/login')
  await page.type('input[autocomplete=username], input[inputmode=email]', EMAIL)
  await page.type('input[type=password]', PASSWORD)
  await page.click('button[type=submit]')
  await page.waitForFunction(() => !location.pathname.startsWith('/login'), { timeout: 15000 })
    .catch(() => { throw new Error('O login não passou: confira o e-mail e a senha (e se o backend está no ar).') })

  for (const [theme, label] of THEMES) {
    await page.evaluate((t) => localStorage.setItem('sidm.theme', t), theme)
    for (const path of pages) {
      try {
        await open(path)
        const v = await check(path, label)
        found.push(...v)
        console.log(`${v.length ? '✗' : '✓'} ${path} (${label})${v.length ? `: ${v.length} regra(s)` : ''}`)
      } catch (e) {
        failed++
        console.log(`! ${path} (${label}): não deu para conferir (${e.message})`)
      }
    }
  }
} finally {
  await browser.close()
}

// junta por regra: a mesma falha costuma aparecer em várias telas (é o mesmo componente)
const byRule = new Map()
for (const v of found) {
  const r = byRule.get(v.id) || { ...v, where: new Set(), targets: new Map() }
  r.where.add(`${v.path} (${v.theme})`)
  for (const n of v.nodes.slice(0, 3)) {
    // o motivo do primeiro teste que falhou (no contraste, diz as cores e a razão)
    const why = [...n.any, ...n.all, ...n.none].find((c) => c.message)?.message || ''
    if (!r.targets.has(n.target.join(' '))) r.targets.set(n.target.join(' '), why)
  }
  byRule.set(v.id, r)
}

console.log('')
if (!byRule.size) {
  console.log(failed ? `Nenhum problema nas telas conferidas (${failed} não abriram).` : 'Nenhum problema encontrado.')
  process.exit(failed ? 1 : 0)
}
const rules = [...byRule.values()].sort((a, b) => ORDER.indexOf(a.impact) - ORDER.indexOf(b.impact))
console.log(`${rules.length} regra(s) com problema:\n`)
for (const r of rules) {
  console.log(`[${IMPACT[r.impact] || r.impact}] ${r.id}: ${r.help}`)
  const where = [...r.where]
  console.log(`  telas (${where.length}): ${where.slice(0, 4).join(', ')}${where.length > 4 ? ` e mais ${where.length - 4}` : ''}`)
  for (const [t, why] of [...r.targets].slice(0, 6)) console.log(`  elemento: ${t}${why ? `\n    ${why}` : ''}`)
  console.log(`  como corrigir: ${r.helpUrl}\n`)
}
process.exit(1)
