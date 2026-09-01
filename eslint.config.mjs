// Root ESLint flat config.
//
// Scope: loose JavaScript at the repo root only (build scripts, tooling
// helpers, container init). Every workspace with its own JS/TS toolchain is
// ignored here and owns its own eslint.config.* — see ui/eslint.config.js.
// documentation/ is Antora (no JS to lint) and documentation-theme/ carries
// its own legacy gulp + eslint-config-standard stack that must not be touched.

import js from '@eslint/js'
import { defineConfig, globalIgnores } from 'eslint/config'
import globals from 'globals'

export default defineConfig([
  globalIgnores([
    'node_modules/**',
    '**/dist/**',
    'ui/**',
    'documentation/**',
    'documentation-theme/**',
  ]),
  {
    files: ['**/*.{js,mjs,cjs}'],
    extends: [js.configs.recommended],
    languageOptions: {
      ecmaVersion: 'latest',
      sourceType: 'module',
      globals: {
        ...globals.node,
      },
    },
  },
  {
    // Runs under mongosh (docker-entrypoint-initdb.d), not Node.
    files: ['config/mongo-init.js'],
    languageOptions: {
      globals: {
        ...globals.mongo,
      },
    },
  },
])
