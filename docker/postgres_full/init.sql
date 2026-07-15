CREATE TABLE people (
  id INT PRIMARY KEY,
  last_name TEXT NOT NULL,
  first_name TEXT NOT NULL,
  middle_name TEXT NOT NULL,
  full_name TEXT GENERATED ALWAYS AS (last_name || ' ' || first_name || ' ' || middle_name) STORED
);

COPY people (id,last_name,first_name,middle_name)
  FROM '/docker-entrypoint-initdb.d/people.csv' WITH CSV;

CREATE INDEX idx_people_fts
  ON people USING GIN (to_tsvector('russian', full_name));

ANALYZE people;
