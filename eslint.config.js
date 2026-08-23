import js from '@eslint/js'
import tseslint from 'typescript-eslint'

// Base ESLint config shared by all workspaces
export const baseConfig = [
  { ignores: ['node_modules/**', 'dist/**', 'build/**', '.vite/**'] },
  {
    files: ['**/*.{js,mjs,cjs,jsx,ts,tsx}'],
    extends: [
      js.configs.recommended,
      ...tseslint.configs.recommended,
    ],
    languageOptions: {
      ecmaVersion: 2022,
      sourceType: 'module',
    },
  },
]

// Standard config for non-React TypeScript code
export const standardConfig = [
  ...baseConfig,
  {
    files: ['**/*.{js,mjs,cjs,ts}'],
    rules: {
      'arrow-parens': ['error', 'always'],
      'comma-dangle': ['error', {
        arrays: 'always-multiline',
        objects: 'always-multiline',
        imports: 'always-multiline',
        exports: 'always-multiline',
      }],
      'no-restricted-properties': ['error', {
        property: 'substr',
        message: 'Use String#slice instead.',
      }],
      'max-len': ['warn', 120, 2],
      'spaced-comment': 'off',
      radix: ['error', 'always'],
    },
  },
]

export default baseConfig
