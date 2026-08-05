-- Lazada's mmstat endpoints are hidden email engagement pixels, not product
-- media. Keep them blocked in the renderer and remove only their stale UI
-- warning fragments, preserving warnings for every other host.
UPDATE google_drive_email_exports AS exports
SET render_warning = COALESCE(
        (
            SELECT string_agg(fragment.warning, ' · ' ORDER BY fragment.ordinality)
            FROM unnest(string_to_array(exports.render_warning, ' · '))
                WITH ORDINALITY AS fragment(warning, ordinality)
            WHERE fragment.warning NOT ILIKE '%mmstat.com%'
        ),
        ''
    ),
    updated_at = NOW()
WHERE exports.render_warning ILIKE '%mmstat.com%';
