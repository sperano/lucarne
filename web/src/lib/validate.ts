import Ajv2020, { type ValidateFunction } from 'ajv/dist/2020.js';
import schema from '../../../contract/v1/transcript.schema.json';
import type { DocumentName, Documents } from './contract';

const ajv = new Ajv2020({ allErrors: true, strict: true });
ajv.addSchema(schema);

const validators = new Map<DocumentName, ValidateFunction>();

function validatorFor(name: DocumentName): ValidateFunction {
  let validate = validators.get(name);
  if (!validate) {
    const compiled = ajv.getSchema(`${schema.$id}#/$defs/${name}`);
    if (!compiled) throw new Error(`no schema definition ${name}`);
    validate = compiled;
    validators.set(name, validate);
  }
  return validate;
}

/** The schema errors of a document, empty when it is valid. */
export function schemaErrors(name: DocumentName, document: unknown): string[] {
  const validate = validatorFor(name);
  if (validate(document)) return [];
  return (validate.errors ?? []).map((e) => `${e.instancePath || '/'} ${e.message ?? e.keyword}`);
}

/** Narrows a decoded document to its contract type, or throws. */
export function parseDocument<N extends DocumentName>(name: N, document: unknown): Documents[N] {
  const errors = schemaErrors(name, document);
  if (errors.length > 0) throw new Error(`invalid ${name}: ${errors.join('; ')}`);
  return document as Documents[N];
}
