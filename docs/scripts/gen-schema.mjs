#!/usr/bin/env node
// Render docs/public/angee.schema.json as a single-page Markdown reference.
// The schema is small and angee-owned, so we walk $defs ourselves rather than
// pull in a heavy generator that fights our $id layout.

import { readFileSync, writeFileSync, mkdirSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'

const here = dirname(fileURLToPath(import.meta.url))
const docsRoot = resolve(here, '..')
const schemaPath = resolve(docsRoot, 'public/angee.schema.json')
const outPath = resolve(docsRoot, 'reference/manifest-schema.md')

const schema = JSON.parse(readFileSync(schemaPath, 'utf8'))
const defs = schema.$defs ?? {}

function refName(ref) {
  if (typeof ref !== 'string') return null
  const m = ref.match(/#\/\$defs\/(.+)$/)
  return m ? m[1] : null
}

function fmtType(prop) {
  if (!prop) return '—'
  if (prop.$ref) {
    const name = refName(prop.$ref)
    return name ? `[${name}](#${name.toLowerCase()})` : '`object`'
  }
  if (prop.type === 'array') {
    const items = prop.items ?? {}
    const inner = items.$ref ? fmtType(items) : items.type ? `\`${items.type}\`` : '`any`'
    return `array<${inner}>`
  }
  if (prop.enum) return prop.enum.map((v) => `\`${v}\``).join(' \\| ')
  if (prop.type) return Array.isArray(prop.type) ? prop.type.map((t) => `\`${t}\``).join(' \\| ') : `\`${prop.type}\``
  return '`object`'
}

function escape(s) {
  if (s == null) return ''
  return String(s).replace(/\|/g, '\\|').replace(/\n/g, ' ').trim()
}

function renderDef(name, def) {
  const lines = [`## ${name}`]
  if (def.description) lines.push(escape(def.description))
  if (def.type) lines.push(`Type: \`${def.type}\``)
  const required = new Set(def.required ?? [])
  const props = def.properties ?? {}
  const propNames = Object.keys(props)
  if (propNames.length > 0) {
    lines.push('')
    lines.push('| Property | Type | Required | Description |')
    lines.push('| --- | --- | --- | --- |')
    for (const p of propNames) {
      const pp = props[p]
      lines.push(
        `| \`${p}\` | ${fmtType(pp)} | ${required.has(p) ? 'yes' : 'no'} | ${escape(pp.description)} |`,
      )
    }
  }
  if (def.additionalProperties && typeof def.additionalProperties === 'object') {
    lines.push('')
    lines.push(`Additional properties: ${fmtType(def.additionalProperties)}.`)
  }
  return lines.join('\n')
}

const sections = []
sections.push('# Manifest schema reference')
sections.push(
  '> Auto-generated from [`docs/public/angee.schema.json`](/angee.schema.json) on every build. For an editorial walkthrough of the manifest see [Manifest](/guide/manifest).',
)

const rootRef = refName(schema.$ref) ?? 'Stack'
const order = [rootRef, ...Object.keys(defs).filter((n) => n !== rootRef).sort()]
for (const name of order) {
  if (!defs[name]) continue
  sections.push(renderDef(name, defs[name]))
}

sections.push('## Raw schema')
sections.push('::: details Click to expand the full JSON Schema')
sections.push('```json')
sections.push(JSON.stringify(schema, null, 2))
sections.push('```')
sections.push(':::')

mkdirSync(dirname(outPath), { recursive: true })
writeFileSync(outPath, sections.join('\n\n') + '\n', 'utf8')
console.log(`wrote ${outPath}`)
