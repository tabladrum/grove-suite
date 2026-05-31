-- 002 - Payload column for L6 (risk heatmap, JSON-LD, replay metadata).
-- payload_json is NOT part of the signed canonical bytes — it carries
-- informational extensions only (risk_heatmap, risk_model_version, etc.).
ALTER TABLE certificates ADD COLUMN payload_json TEXT NOT NULL DEFAULT '{}';
