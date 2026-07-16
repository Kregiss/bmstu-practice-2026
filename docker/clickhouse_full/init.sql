CREATE TABLE people
(
    id UInt64,
    last_name String,
    first_name String,
    middle_name String,
    full_name String
)
ENGINE = MergeTree()
ORDER BY id;

INSERT INTO people
SELECT
    id,
    last_name,
    first_name,
    middle_name,
    concat(last_name, ' ', first_name, ' ', middle_name) as full_name
FROM file('/data/people.csv', 'CSV');

ALTER TABLE people
ADD INDEX idx_token full_name TYPE tokenbf_v1(32768, 3, 0) GRANULARITY 1;

ALTER TABLE people MATERIALIZE INDEX idx_token;
