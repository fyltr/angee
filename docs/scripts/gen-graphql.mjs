#!/usr/bin/env node
// Generate Markdown reference pages from internal/operator/schema.graphql.
// Driven by @graphql-markdown/cli (`gqlmd`) reading docs/.graphqlrc.yml.
// graphql-markdown writes a homepage at `<baseURL>/<homepage-basename>.md`;
// we rename it to index.md so `/reference/graphql/` resolves directly.

import { execSync } from 'node:child_process'
import { renameSync, rmSync, existsSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve, join } from 'node:path'

const here = dirname(fileURLToPath(import.meta.url))
const docsRoot = resolve(here, '..')
const outDir = resolve(docsRoot, 'reference/graphql')

rmSync(outDir, { recursive: true, force: true })

execSync('npx --no-install gqlmd graphql-to-doc default', {
  stdio: 'inherit',
  cwd: docsRoot,
})

const generatedHome = join(outDir, 'graphql-homepage.md')
const indexPath = join(outDir, 'index.md')
if (existsSync(generatedHome)) {
  renameSync(generatedHome, indexPath)
}

console.log(`graphql-markdown wrote pages to ${outDir}`)
