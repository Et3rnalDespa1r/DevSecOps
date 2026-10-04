BEGIN;

CREATE TABLE IF NOT EXISTS inventory_items (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    inventory_number text NOT NULL UNIQUE CHECK (btrim(inventory_number) <> ''),
    name text NOT NULL CHECK (btrim(name) <> ''),
    condition_description text NOT NULL,
    usable boolean NOT NULL DEFAULT true
);

COMMIT;
