-- Store the raw OpenAPI/Swagger spec (JSON or YAML) for a product so the
-- catalog can render interactive docs + Try-it. Empty = no docs.
ALTER TABLE api_products ADD COLUMN IF NOT EXISTS openapi_spec TEXT NOT NULL DEFAULT '';
