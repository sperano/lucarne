import js from '@eslint/js';
import svelte from 'eslint-plugin-svelte';
import globals from 'globals';
import ts from 'typescript-eslint';

// Code that runs under Node rather than in the browser: tooling configs and
// the Vitest suites. tsconfig.node.json lists the same files.
const NODE_FILES = ['*.config.{js,ts}', 'src/**/*.test.ts'];

export default ts.config(
  { ignores: ['dist/', 'node_modules/'] },
  js.configs.recommended,
  ...ts.configs.strict,
  ...svelte.configs.recommended,
  { languageOptions: { globals: globals.browser } },
  { files: NODE_FILES, languageOptions: { globals: globals.node } },
  {
    files: ['**/*.svelte', '**/*.svelte.ts'],
    languageOptions: { parserOptions: { parser: ts.parser } },
  },
);
