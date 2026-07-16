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
CREATE INDEX idx_people_fts
  ON people USING GIN (to_tsvector('russian', full_name));
