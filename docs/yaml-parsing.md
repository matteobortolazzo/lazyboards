# YAML Parsing with yaml.v3

Custom `UnmarshalYAML` implementations in the `internal/config` package.

## Rules

- When implementing `UnmarshalYAML` on a type that could receive a `!!null` scalar node, explicitly check `valueNode.Tag == "!!null"` **before** calling `node.Decode(&target)`. The yaml.v3 library short-circuits `Decode` for null nodes, invoking the zero-value path directly and bypassing your custom `UnmarshalYAML` handler entirely. Place the null-tag check in the caller (e.g., `parseKeymapTable`) as the primary guard; also add a secondary null-tag check inside `UnmarshalYAML` itself as defense-in-depth for other decode paths that might reach it differently.
