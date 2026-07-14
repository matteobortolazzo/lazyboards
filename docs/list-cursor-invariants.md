# List-Cursor Invariants

When a code path mutates `b.Columns[ci].Cards` (the card list of a column), the cursor must remain in valid bounds `[0, len(Cards)-1]` before `View()` is called. Failing to clamp allows out-of-bounds panics in view renderers like `viewCardDetail` which index `col.Cards[col.Cursor]`.

## Rules

- After any splice or replacement of `b.Columns[ci].Cards`, immediately clamp `b.Columns[ci].Cursor` to the new list length: `if col.Cursor >= len(col.Cards) { col.Cursor = len(col.Cards) - 1; if col.Cursor < 0 { col.Cursor = 0 } }`. This applies to all card-removal scenarios (background fetch, user action) consistently.
- Example precedents: `handleBoardFetched` (line ~429, refresh discards old cards) and `handleCardClosed` (line ~759, user closes a card). Both use the identical clamp pattern — do not invent variations.
- Do not assume that navigation guards alone (e.g., `Cursor < len(Cards)` checks in key handlers) protect View renderers; View renderers may be called from other paths where the cursor was not re-validated. Always enforce the invariant at the mutation site.
