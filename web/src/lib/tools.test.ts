import { describe, expect, it } from 'vitest';
import { describeTool } from './tools';

describe('describeTool', () => {
  it('shows a command apart from the rest of the input', () => {
    const d = describeTool({
      callId: 'c',
      name: 'Bash',
      input: { command: 'go test ./...\ngo vet ./...', description: 'Run the tests' },
    });
    expect(d).toEqual({
      subject: 'go test ./...',
      command: 'go test ./...\ngo vet ./...',
      inputJSON: '{\n  "description": "Run the tests"\n}',
    });
    expect(
      describeTool({ callId: 'c', name: 'bash', input: { command: 'ls' } }).inputJSON,
    ).toBeUndefined();
  });

  it('names a file or a query when there is no command', () => {
    expect(
      describeTool({ callId: 'c', name: 'Read', input: { file_path: '/w/a.go' } }).subject,
    ).toBe('/w/a.go');
    expect(describeTool({ callId: 'c', name: 'read', input: { filePath: '/w/b.go' } }).path).toBe(
      '/w/b.go',
    );
    expect(describeTool({ callId: 'c', name: 'read', title: 'b.go', input: {} }).subject).toBe(
      'b.go',
    );
    expect(
      describeTool({ callId: 'c', name: 'web_search', input: { query: 'example' } }).subject,
    ).toBe('example');
    expect(
      describeTool({ callId: 'c', name: 'x', input: { command: '' } }).command,
    ).toBeUndefined();
  });

  it('reads a to-do list', () => {
    const d = describeTool({
      callId: 'c',
      name: 'todo_list',
      input: {
        items: [
          { text: 'Run the tests', completed: true },
          { content: 'Fix it', status: 'pending' },
        ],
      },
    });
    expect(d.todos).toEqual([
      { text: 'Run the tests', done: true },
      { text: 'Fix it', done: false },
    ]);
    expect(
      describeTool({ callId: 'c', name: 'x', input: { items: [1, 2] } }).todos,
    ).toBeUndefined();
  });

  it('shows input that is not an object as JSON, and no input as nothing', () => {
    expect(describeTool({ callId: 'c', name: 'x', input: ['a'] })).toEqual({
      subject: '',
      inputJSON: '[\n  "a"\n]',
    });
    expect(describeTool({ callId: 'c', name: 'x', inputText: '{"cut' })).toEqual({ subject: '' });
  });
});
