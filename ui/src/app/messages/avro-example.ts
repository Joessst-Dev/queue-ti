interface AvroSchema {
  type?: unknown;
  symbols?: string[];
  items?: unknown;
  values?: unknown;
  fields?: Array<{ name: string; type: unknown }>;
}

function normalise(schema: unknown): AvroSchema {
  if (typeof schema === 'string') {
    return { type: schema };
  }
  if (Array.isArray(schema)) {
    const nonNull = schema.find((s) => s !== 'null');
    return nonNull !== undefined ? normalise(nonNull) : { type: 'null' };
  }
  return schema as AvroSchema;
}

export function generateAvroExample(schema: unknown, depth = 0): unknown {
  if (depth > 10) {
    return null;
  }

  const norm = normalise(schema);

  switch (norm.type) {
    case 'null':
      return null;
    case 'boolean':
      return false;
    case 'int':
    case 'long':
      return 0;
    case 'float':
    case 'double':
      return 0.0;
    case 'string':
    case 'bytes':
      return '';
    case 'enum':
      return norm.symbols?.[0] ?? '';
    case 'array':
      return [generateAvroExample(norm.items, depth + 1)];
    case 'map':
      return { key: generateAvroExample(norm.values, depth + 1) };
    case 'record': {
      const result: Record<string, unknown> = {};
      for (const field of norm.fields ?? []) {
        result[field.name] = generateAvroExample(field.type, depth + 1);
      }
      return result;
    }
    default:
      return null;
  }
}
