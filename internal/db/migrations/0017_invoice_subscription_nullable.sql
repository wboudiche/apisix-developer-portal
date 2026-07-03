ALTER TABLE invoices ALTER COLUMN subscription_id DROP NOT NULL;
ALTER TABLE invoices DROP CONSTRAINT invoices_subscription_id_fkey;
ALTER TABLE invoices ADD CONSTRAINT invoices_subscription_id_fkey
  FOREIGN KEY (subscription_id) REFERENCES subscriptions(id) ON DELETE SET NULL;
