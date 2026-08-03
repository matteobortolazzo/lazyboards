# YAML Parsing with yaml.v3

Custom `UnmarshalYAML` implementations in the `internal/config` package.

## Rules

- When implementing `UnmarshalYAML` on a type that could receive a `!!null` scalar node, explicitly check `valueNode.Tag == "!!null"` **before** calling `node.Decode(&target)`. The yaml.v3 library short-circuits `Decode` for null nodes, invoking the zero-value path directly and bypassing your custom `UnmarshalYAML` handler entirely. Place the null-tag check in the caller (e.g., `parseKeymapTable`) as the primary guard; also add a secondary null-tag check inside `UnmarshalYAML` itself as defense-in-depth for other decode paths that might reach it differently.
- Never use a raw-node walk (iterating `yaml.Node.Content` by hand and checking literal `ValueNode.Value` strings) to make a security-relevant decision (provenance/trust gate, format validation, origin detection, etc.) when a full `yaml.v3` decode of the same input is also happening in the same code path. A raw-node walk and yaml.v3's full generic map decoder can diverge on YAML features like merge keys (`<<: *anchor`), aliases (`*anchor`), and anchor resolution — the decoder expands these but a hand-rolled walk does not. This means a security gate based on "was this key mentioned literally in the YAML" can be bypassed by using merge keys or aliases. Always derive security-relevant decisions from the decoded state instead (e.g., value-comparison against a snapshot of the decoded global config) — the decoded object is what the rest of the system actually sees and acts on, so it must be your source of truth.
