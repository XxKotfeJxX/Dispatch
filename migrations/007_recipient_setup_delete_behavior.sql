ALTER TABLE recipient_setups
    DROP CONSTRAINT IF EXISTS recipient_setups_recipient_id_fkey;

ALTER TABLE recipient_setups
    ADD CONSTRAINT recipient_setups_recipient_id_fkey
    FOREIGN KEY (recipient_id) REFERENCES recipients(id) ON DELETE SET NULL;
