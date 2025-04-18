CREATE TABLE IF NOT EXISTS compound (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  scale TEXT CHECK(scale IN ('mg', 'ml'))
);

CREATE TABLE IF NOT EXISTS quantity (
  id TEXT PRIMARY KEY,
  num_of_units INT NOT NULL,
  quantity_per_unit INT NOT NULL
);

CREATE TABLE IF NOT EXISTS entry (
  id TEXT PRIMARY KEY,
  type TEXT NOT NULL CHECK(type IN ('incoming', 'outgoing')),
  compound_id TEXT NOT NULL,
  date INT NOT NULL,
  remark TEXT,
  voucher_no TEXT,
  quantity_id TEXT NOT NULL,
  net_stock INT NOT NULL,
  FOREIGN KEY(compound_id) REFERENCES compound(id),
  FOREIGN KEY(quantity_id) REFERENCES quantity(id)
);