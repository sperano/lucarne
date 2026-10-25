import js from '@eslint/js';
import svelte from 'eslint-plugin-svelte';
import globals from 'globals';
import ts from 'typescript-eslint';

// Code that runs under Node rather than in the browser: tooling configs and
// the Vitest suites. tsconfig.node.json lists the same files.
const NODE_FILES = ['*.config.{js,ts}', 'src/**/*.test.ts'];

// The schema validator, which runs Ajv and so needs eval; see its header.
const VALIDATOR = 'src/lib/validate.ts';

// The TypeScript projects the type-aware rules read: the app, then the Node
// files (tests and the TypeScript configs).
const TS_PROJECTS = ['./tsconfig.json', './tsconfig.node.json'];

export default ts.config(
  { ignores: ['dist/', 'node_modules/'] },
  js.configs.recommended,
  ...ts.configs.strictTypeChecked,
  ...svelte.configs.recommended,
  {
    languageOptions: {
      globals: globals.browser,
      parserOptions: {
        project: TS_PROJECTS,
        tsconfigRootDir: import.meta.dirname,
        extraFileExtensions: ['.svelte'],
      },
    },
  },
  // Plain JavaScript configs: not in a TypeScript program, so no type information.
  { files: ['**/*.js'], ...ts.configs.disableTypeChecked },
  { files: NODE_FILES, languageOptions: { globals: globals.node } },
  {
    files: ['src/**/*.{ts,svelte}'],
    ignores: [...NODE_FILES, VALIDATOR],
    rules: {
      'no-restricted-imports': [
        'error',
        {
          patterns: [
            {
              regex: '(^|/)validate(\\.ts)?$|^ajv(/|$)',
              message:
                'Ajv needs eval, which the CSP forbids: tests only (see src/lib/validate.ts).',
            },
          ],
        },
      ],
    },
  },
  {
    files: ['**/*.svelte', '**/*.svelte.ts'],
    languageOptions: { parserOptions: { parser: ts.parser } },
  },
);
