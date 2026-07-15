CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE people (
  id INT PRIMARY KEY,
  last_name TEXT NOT NULL,
  first_name TEXT NOT NULL,
  middle_name TEXT NOT NULL,
  full_name TEXT GENERATED ALWAYS AS (last_name || ' ' || first_name || ' ' || middle_name) STORED
);

COPY people (id,last_name,first_name,middle_name)
  FROM '/docker-entrypoint-initdb.d/people.csv' WITH CSV;

CREATE INDEX idx_people_trgm_knn
  ON people USING GIST (full_name gist_trgm_ops(siglen=64));

ANALYZE people;
