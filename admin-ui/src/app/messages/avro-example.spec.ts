import { describe, it, expect } from 'vitest';
import { generateAvroExample } from './avro-example';

describe('generateAvroExample', () => {
  describe('when given primitive type "null"', () => {
    it('should return null', () => {
      expect(generateAvroExample('null')).toBeNull();
    });
  });

  describe('when given primitive type "boolean"', () => {
    it('should return false', () => {
      expect(generateAvroExample('boolean')).toBe(false);
    });
  });

  describe('when given primitive type "int"', () => {
    it('should return 0', () => {
      expect(generateAvroExample('int')).toBe(0);
    });
  });

  describe('when given primitive type "long"', () => {
    it('should return 0', () => {
      expect(generateAvroExample('long')).toBe(0);
    });
  });

  describe('when given primitive type "float"', () => {
    it('should return 0', () => {
      expect(generateAvroExample('float')).toBe(0);
    });
  });

  describe('when given primitive type "double"', () => {
    it('should return 0', () => {
      expect(generateAvroExample('double')).toBe(0);
    });
  });

  describe('when given primitive type "string"', () => {
    it('should return empty string', () => {
      expect(generateAvroExample('string')).toBe('');
    });
  });

  describe('when given primitive type "bytes"', () => {
    it('should return empty string', () => {
      expect(generateAvroExample('bytes')).toBe('');
    });
  });

  describe('when given an enum schema', () => {
    it('should return the first symbol', () => {
      const schema = { type: 'enum', name: 'Status', symbols: ['ACTIVE', 'INACTIVE'] };
      expect(generateAvroExample(schema)).toBe('ACTIVE');
    });

    it('should return empty string when symbols array is empty', () => {
      const schema = { type: 'enum', name: 'Empty', symbols: [] };
      expect(generateAvroExample(schema)).toBe('');
    });
  });

  describe('when given an array schema', () => {
    it('should return a one-element array with the items example', () => {
      const schema = { type: 'array', items: 'string' };
      expect(generateAvroExample(schema)).toEqual(['']);
    });
  });

  describe('when given a map schema', () => {
    it('should return an object with key "key" mapped to the values example', () => {
      const schema = { type: 'map', values: 'int' };
      expect(generateAvroExample(schema)).toEqual({ key: 0 });
    });
  });

  describe('when given a flat record schema', () => {
    it('should return an object with each field mapped to its example value', () => {
      const schema = {
        type: 'record',
        name: 'Order',
        fields: [
          { name: 'id', type: 'string' },
          { name: 'amount', type: 'double' },
        ],
      };
      expect(generateAvroExample(schema)).toEqual({ id: '', amount: 0 });
    });
  });

  describe('when given a nested record schema', () => {
    it('should recursively generate examples for nested record fields', () => {
      const schema = {
        type: 'record',
        name: 'Order',
        fields: [
          { name: 'id', type: 'string' },
          {
            name: 'address',
            type: {
              type: 'record',
              name: 'Address',
              fields: [
                { name: 'street', type: 'string' },
                { name: 'zip', type: 'string' },
              ],
            },
          },
        ],
      };
      expect(generateAvroExample(schema)).toEqual({
        id: '',
        address: { street: '', zip: '' },
      });
    });
  });

  describe('when given a union ["null", "string"]', () => {
    it('should return the example for the non-null branch', () => {
      expect(generateAvroExample(['null', 'string'])).toBe('');
    });
  });

  describe('when given a union ["null"]', () => {
    it('should return null', () => {
      expect(generateAvroExample(['null'])).toBeNull();
    });
  });

  describe('when given a union ["null", record]', () => {
    it('should return the record example', () => {
      const schema = [
        'null',
        {
          type: 'record',
          name: 'Event',
          fields: [{ name: 'name', type: 'string' }],
        },
      ];
      expect(generateAvroExample(schema)).toEqual({ name: '' });
    });
  });

  describe('when given an unknown type', () => {
    it('should return null', () => {
      expect(generateAvroExample({ type: 'unknownType' })).toBeNull();
    });
  });

  describe('when given an empty object', () => {
    it('should return null', () => {
      expect(generateAvroExample({})).toBeNull();
    });
  });

  describe('depth guard', () => {
    it('should return null when depth exceeds 10', () => {
      // Build a chain of 12 nested records (L1 → L2 → ... → L12).
      // L12 is at depth 11 when reached from the top, so the guard returns null for it.
      let schema: unknown = { type: 'record', name: 'L12', fields: [{ name: 'v', type: 'string' }] };
      for (let i = 11; i >= 1; i--) {
        schema = { type: 'record', name: `L${i}`, fields: [{ name: 'child', type: schema }] };
      }
      // At depth 0 we have L1. Following 11 'child' hops reaches depth 11 → null.
      const result = generateAvroExample(schema) as Record<string, unknown>;
      let current: unknown = result;
      for (let i = 0; i < 11; i++) {
        current = (current as Record<string, unknown>)['child'];
      }
      expect(current).toBeNull();
    });
  });
});
