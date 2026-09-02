import js from '@eslint/js';
import tseslint from 'typescript-eslint';
import prettierConfig from 'eslint-config-prettier';

export default tseslint.config(
  {
    ignores: ['**/dist/**', '**/build/**', '**/node_modules/**', '**/.next/**', '**/coverage/**'],
  },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  prettierConfig,
  {
    rules: {
      '@typescript-eslint/no-unused-vars': ['error', { argsIgnorePattern: '^_' }],
      '@typescript-eslint/no-explicit-any': 'warn',
    },
  },
  {
    // node-pg-migrate требует CommonJS-файлы миграций (exports.up/exports.down).
    files: ['**/migrations/**/*.cjs'],
    languageOptions: {
      globals: { exports: 'writable', module: 'writable', require: 'readonly' },
    },
  },
  {
    // Конфиги (next.config.mjs, postcss.config.mjs) выполняются в Node, а не в браузере.
    files: ['**/*.config.mjs'],
    languageOptions: {
      globals: { process: 'readonly', URL: 'readonly' },
    },
  },
);
