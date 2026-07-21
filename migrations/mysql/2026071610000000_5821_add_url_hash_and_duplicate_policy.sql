-- +goose Up
ALTER TABLE `short_links`
  ADD COLUMN `url_hash` VARCHAR(64) NOT NULL DEFAULT '' AFTER `original_url`;

CREATE INDEX `idx_short_links_ws_domain_urlhash`
  ON `short_links` (`workspace_id`, `domain_id`, `url_hash`);

ALTER TABLE `domains`
  ADD COLUMN `duplicate_policy` VARCHAR(20) NOT NULL DEFAULT 'by_request' AFTER `default_start_number`;

-- +goose Down
ALTER TABLE `domains` DROP COLUMN `duplicate_policy`;

DROP INDEX `idx_short_links_ws_domain_urlhash` ON `short_links`;

ALTER TABLE `short_links` DROP COLUMN `url_hash`;
