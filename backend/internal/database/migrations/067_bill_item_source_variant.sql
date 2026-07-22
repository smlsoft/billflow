-- Preserve the source marketplace variant and line order so duplicate product
-- rows can be traced back to the correct email product block.
ALTER TABLE bill_items
  ADD COLUMN IF NOT EXISTS source_variant TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS source_line_no INTEGER NOT NULL DEFAULT 0;
