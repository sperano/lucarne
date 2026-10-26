// What the conversation shows of a tool call, read from its input by the
// field names the runners use rather than by runner: Claude Code's Bash and
// OpenCode's bash both carry `command`, Claude's file tools `file_path`,
// OpenCode's `filePath`. Input with none of them is shown as JSON only.
import type { ToolCall } from './contract';

const COMMAND_KEYS = ['command', 'cmd'] as const;
const PATH_KEYS = ['file_path', 'filePath', 'path', 'notebook_path'] as const;
const QUERY_KEYS = ['description', 'query', 'pattern', 'url', 'prompt'] as const;
const TODO_KEYS = ['items', 'todos'] as const;
const TODO_TEXT_KEYS = ['text', 'content'] as const;
const DONE_STATUS = 'completed';
const JSON_INDENT = 2;

export interface Todo {
  readonly text: string;
  readonly done: boolean;
}

export interface ToolDescription {
  /** One line naming what the call does, '' when its input names nothing. */
  readonly subject: string;
  readonly command?: string;
  readonly path?: string;
  readonly todos?: readonly Todo[];
  /** The input as indented JSON, without the command shown above it; absent when nothing is left. */
  readonly inputJSON?: string;
}

type Fields = Readonly<Record<string, unknown>>;

/** Describes call for its summary line and body. */
export function describeTool(call: ToolCall): ToolDescription {
  const input = isFields(call.input) ? call.input : undefined;
  const commandField = input && firstString(input, COMMAND_KEYS);
  const command = commandField?.value;
  const path = input && firstString(input, PATH_KEYS)?.value;
  const todos = input && todoList(input);
  const query = input && firstString(input, QUERY_KEYS)?.value;
  const subject = firstLine(command) || path || call.title || firstLine(query);
  const inputJSON = jsonWithout(call.input, commandField?.key);
  return {
    subject,
    ...(command === undefined ? {} : { command }),
    ...(path === undefined ? {} : { path }),
    ...(todos === undefined ? {} : { todos }),
    ...(inputJSON === undefined ? {} : { inputJSON }),
  };
}

function isFields(value: unknown): value is Fields {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

/** The first of keys whose value in input is a non-empty string. */
function firstString(
  input: Fields,
  keys: readonly string[],
): { key: string; value: string } | undefined {
  for (const key of keys) {
    const value = input[key];
    if (typeof value === 'string' && value !== '') return { key, value };
  }
  return undefined;
}

function firstLine(text: string | undefined): string {
  if (text === undefined) return '';
  const newline = text.indexOf('\n');
  return (newline === -1 ? text : text.slice(0, newline)).trim();
}

function todoList(input: Fields): Todo[] | undefined {
  for (const key of TODO_KEYS) {
    const value = input[key];
    if (!Array.isArray(value) || value.length === 0) continue;
    const todos = value.map(todo);
    if (todos.every((t) => t !== null)) return todos;
  }
  return undefined;
}

function todo(value: unknown): Todo | null {
  if (!isFields(value)) return null;
  const text = firstString(value, TODO_TEXT_KEYS)?.value;
  if (text === undefined) return null;
  const done = value.completed === true || value.status === DONE_STATUS;
  return { text, done };
}

function jsonWithout(input: unknown, key: string | undefined): string | undefined {
  if (input === undefined) return undefined;
  if (key === undefined || !isFields(input)) return JSON.stringify(input, null, JSON_INDENT);
  const rest = Object.fromEntries(Object.entries(input).filter(([k]) => k !== key));
  return Object.keys(rest).length === 0 ? undefined : JSON.stringify(rest, null, JSON_INDENT);
}
