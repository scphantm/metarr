// ESLint flat config for the Metarr web UI (React 19 + TypeScript + Vite).
//
// Presets only — no hand-picked rule list. Type-aware linting is on
// (recommendedTypeChecked); typescript-eslint v8 does not officially support
// TypeScript 7.x yet, so it prints an "unsupported version" warning on every
// run. That is expected and nothing here suppresses it.

import js from '@eslint/js'
import eslintReact from '@eslint-react/eslint-plugin'
import eslintConfigPrettier from 'eslint-config-prettier'
import globals from 'globals'
import tseslint from 'typescript-eslint'

export default tseslint.config(
  {
    ignores: [
      'dist/**',
      '.vite/**', // Vite's pre-bundle cache
      'coverage/**',
      'node_modules/**',
      'src/gen/**', // protoc-gen-es output — never hand-edited
      '**/*.tsbuildinfo',
    ],
  },

  // Application + test source.
  {
    files: ['**/*.{ts,tsx}'],
    extends: [
      js.configs.recommended,
      tseslint.configs.recommendedTypeChecked,
      eslintReact.configs['recommended-type-checked'],
    ],
    languageOptions: {
      parser: tseslint.parser,
      parserOptions: {
        projectService: true,
        tsconfigRootDir: import.meta.dirname,
      },
      globals: {
        ...globals.browser,
      },
    },
  },

  // Test files: the type-checked presets are noisy against test doubles and
  // fixtures, so relax the rules that fire most there.
  {
    files: [
      '**/*.{test,spec}.{ts,tsx}',
      'src/test/**/*.{ts,tsx}',
      'src/**/__tests__/**/*.{ts,tsx}',
    ],
    rules: {
      '@typescript-eslint/no-explicit-any': 'off',
      '@typescript-eslint/no-non-null-assertion': 'off',
      '@typescript-eslint/no-unsafe-argument': 'off',
      '@typescript-eslint/no-unsafe-assignment': 'off',
      '@typescript-eslint/no-unsafe-call': 'off',
      '@typescript-eslint/no-unsafe-member-access': 'off',
      '@typescript-eslint/no-unsafe-return': 'off',
    },
  },

  // Config files (vite/vitest/eslint) live outside the tsconfig project.
  {
    files: ['**/*.config.{js,ts,mjs,cjs}', 'eslint.config.js'],
    extends: [tseslint.configs.disableTypeChecked],
    languageOptions: {
      globals: {
        ...globals.node,
      },
    },
  },

  // Must stay last: turns off rules that would conflict with Prettier.
  eslintConfigPrettier,
)
