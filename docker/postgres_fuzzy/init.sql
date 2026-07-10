CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE TABLE people (
  id INT PRIMARY KEY,
  last_name TEXT,
  first_name TEXT,
  middle_name TEXT,
  full_name TEXT
);
COPY people (id,last_name,first_name,middle_name)
  FROM '/docker-entrypoint-initdb.d/people.csv' WITH CSV HEADER;
UPDATE people SET full_name = last_name || ' ' || first_name || ' ' || middle_name;
CREATE INDEX idx_people_trgm
  ON people USING GIST (full_name gist_trgm_ops);
