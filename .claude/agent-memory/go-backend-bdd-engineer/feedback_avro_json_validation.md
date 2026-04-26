---
name: Avro JSON validation approach
description: hamba/avro/v2 is binary-only; JSON payloads must be validated with a hand-rolled struct-check, not avro.Unmarshal
type: feedback
---

`github.com/hamba/avro/v2`'s `avro.Unmarshal` and `avro.NewDecoder` decode **Avro binary** encoding only. Calling them on a JSON byte slice (e.g. `{"id":"x","value":1}`) returns `avro: ReadSTRING: invalid string length` even for a perfectly valid payload.

**Why:** queue-ti payloads are JSON strings (submitted over HTTP/gRPC). Requiring producers to send Avro-binary would be a breaking protocol change.

**How to apply:** When validating JSON payloads against an Avro record schema, use a hand-rolled approach:
1. Parse the payload with `encoding/json` into a `map[string]json.RawMessage`.
2. Walk the `avro.RecordSchema.Fields()` slice; for each field without a default, verify it is present.
3. For each present field, check the JSON kind against the Avro type (string→string, int/long→integer number, float/double→number, boolean→bool, record→recurse).
4. `avro.Parse` is still used for schema compilation and caching — only the validation step is custom.
