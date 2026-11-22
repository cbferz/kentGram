# Profile avatar uploads

The Android profile editor uploads JPEG and PNG images to `POST /api/v1/me/avatar`.

- Maximum file size: 5 MiB.
- Maximum dimensions: 4096 × 4096 pixels and 12 megapixels.
- Uploads require the current account's Bearer access token.
- Files are stored in the `kentgram_media` Docker volume at `/data/avatars`.
- Public URLs use `https://api.kentgram.tendercheck.pro/media/avatars/{opaque-id}.{jpg|png}`.
- Public files are immutable and cached for one year. A new upload creates a new URL; existing URLs remain valid.

The database schema is unchanged. The API's existing username-login flow is included in the stacked change set. Only the current user's `avatar_url` profile field is updated after the file is safely written.
