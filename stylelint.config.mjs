// Base stylelint config shared by all workspaces
export const baseConfig = {
  extends: ['stylelint-config-standard'],
}

// UI-specific config with Tailwind CSS support
export const uiConfig = {
  extends: [
    'stylelint-config-standard',
    'stylelint-config-tailwindcss',
  ],
}

// Documentation theme config with relaxed rules
export const themeConfig = {
  extends: 'stylelint-config-standard',
  rules: {
    'comment-empty-line-before': null,
    'no-descending-specificity': null,
  },
}

export default baseConfig
